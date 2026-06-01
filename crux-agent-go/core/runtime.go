package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hermes-go/core/agentloop"
	"github.com/hermes-go/core/bus"
	"github.com/hermes-go/core/config"
	ctxmgr "github.com/hermes-go/core/context"
	"github.com/hermes-go/core/guardrails"
	"github.com/hermes-go/core/llm"
	"github.com/hermes-go/core/primitives"
	"github.com/hermes-go/core/store"
	"github.com/hermes-go/core/types"
	"github.com/hermes-go/skills/compaction"
	"github.com/hermes-go/skills/memory"
	"github.com/hermes-go/skills/terminal"
)

// SkillEntry represents a dynamically loaded skill.
type SkillEntry struct {
	Name    string
	Tools   []types.ToolSchema
	Handler func(types.ToolCall) types.ToolResult
}

// Runtime assembles all components and runs the agent.
type Runtime struct {
	cfg         *config.Config
	provider    llm.Provider
	loop        *agentloop.Loop
	ctxMgr      *ctxmgr.Manager
	store       store.Store
	bus         *bus.Bus
	memory      *memory.Skill
	guard       *guardrails.Guardrails
	compactor   *compaction.Compactor
	logger      *slog.Logger
	homeDir     string
	sessionID   string
	skills      []SkillEntry
	allTools    []types.ToolSchema
	allHandlers map[string]func(types.ToolCall) types.ToolResult
}

func New(cfg *config.Config) (*Runtime, error) {
	rt := &Runtime{cfg: cfg, logger: slog.Default()}

	home, _ := os.UserHomeDir()
	rt.homeDir = home

	// 1. Store
	s, err := store.NewSQLite(cfg.Store.DSN)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	rt.store = s

	// 2. Event Bus
	rt.bus = bus.New()

	// 3. LLM Provider
	rt.provider = llm.NewOpenAI(cfg.LLM.APIKey, cfg.LLM.BaseURL, cfg.LLM.Model)

	// 4. Memory skill
	memDir := filepath.Join(home, ".hermes", "memory")
	rt.memory = memory.New(memDir)
	if err := rt.memory.Init(nil); err != nil {
		rt.logger.Warn("memory init failed", "error", err)
	}

	// 5. Guardrails
	rt.guard = guardrails.New(guardrails.Config{
		MaxInputLength:  100000,
		MaxOutputLength: 50000,
		BlockedCommands: []string{"rm -rf /", "dd if=", "mkfs"},
		RateLimit:       60,
	})

	// 6. Compaction
	rt.compactor = compaction.New(rt.provider).
		WithStore(rt.store).
		WithStrategy(compaction.StrategyTwoStage)

	// 7. Register all tools
	rt.allHandlers = make(map[string]func(types.ToolCall) types.ToolResult)

	// File primitives with path sandbox
	workDir, _ := os.Getwd()
	fp := primitives.NewFileSandbox([]string{workDir, home, "/tmp"})
	rt.registerToolSet(fp.ToolSchemas(), fp.Handle, []string{
		"read_file", "write_file", "patch_file", "search_content", "search_files",
	})

	// System primitives
	sp := primitives.NewSystem()
	rt.registerToolSet(sp.ToolSchemas(), sp.Handle, []string{
		"get_time", "get_os", "get_cwd", "get_env",
	})

	// Terminal skill (with timeout)
	ts := terminal.New()
	ts.Timeout = 30 * time.Second
	ts.WorkDir = workDir
	rt.registerToolSet(ts.ToolSchemas(), ts.Handle, []string{"exec"})

	// Memory skill
	rt.registerToolSet(rt.memory.ToolSchemas(), rt.memory.Handle, []string{
		"memory_save", "memory_search", "memory_list", "memory_delete",
	})

	// 8. Tool executor with guardrails
	executor := func(call types.ToolCall) types.ToolResult {
		// Emit event
		rt.bus.Emit(bus.Event{
			Type:    "tool_call",
			Source:  "agent-loop",
			Payload: call.Function.Name,
		})

		// Guardrails check
		if err := rt.guard.CheckToolCall(call); err != nil {
			return types.ToolResult{
				ToolCallID: call.ID,
				Content:    fmt.Sprintf("Guardrail violation: %v", err),
				IsError:    true,
			}
		}

		fn, ok := rt.allHandlers[call.Function.Name]
		if !ok {
			return types.ToolResult{
				ToolCallID: call.ID,
				Content:    fmt.Sprintf("Unknown tool: %s", call.Function.Name),
				IsError:    true,
			}
		}
		result := fn(call)

		// Output guard
		result = rt.guard.CheckOutput(call.Function.Name, result)
		return result
	}

	// 9. Agent Loop
	rt.loop = agentloop.New(rt.provider, rt.allTools, executor)
	rt.loop.MaxRounds = cfg.LLM.MaxRounds

	// 10. Context Manager
	rt.ctxMgr = ctxmgr.NewWithBudget(cfg.Agent.SystemPrompt, 128000)

	return rt, nil
}

// registerToolSet registers a set of tools and their handler.
func (rt *Runtime) registerToolSet(tools []types.ToolSchema, handler func(types.ToolCall) types.ToolResult, names []string) {
	rt.allTools = append(rt.allTools, tools...)
	for _, name := range names {
		rt.allHandlers[name] = handler
	}
}

// Run starts the interactive CLI loop with graceful shutdown.
func (rt *Runtime) Run(ctx context.Context) error {
	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Create session
	sessID := generateID()
	rt.sessionID = sessID
	rt.compactor.WithSession(sessID)

	sess := &store.SessionMeta{
		ID:        sessID,
		Title:     "CLI Session",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := rt.store.CreateSession(sess); err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	// Set up loop callbacks
	rt.loop.SetPersister(func(msg types.Message) {
		storedMsg := &store.StoredMessage{
			ID:        generateID(),
			SessionID: sessID,
			Role:      msg.Role,
			Content:   msg.Content,
			TokenEst:  ctxmgr.EstimateTokens([]types.Message{msg}),
		}
		if len(msg.ToolCalls) > 0 {
			tc, _ := jsonMarshal(msg.ToolCalls)
			storedMsg.ToolCalls = tc
		}
		storedMsg.ToolCallID = msg.ToolCallID
		storedMsg.Name = msg.Name
		if err := rt.store.AppendMessage(sessID, storedMsg); err != nil {
			rt.logger.Warn("failed to persist message", "error", err)
		}
	})

	rt.loop.SetCompactor(rt.compactor.CompactForLoop())

	// Counter
	var toolCount int
	rt.bus.Subscribe("tool_call", "counter", func(e bus.Event) error {
		toolCount++
		return nil
	})

	// Inject memory into context
	rt.ctxMgr.SetMemory(rt.memory.GetMemoryText())

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	fmt.Printf("Hermes (%s) ready. Model: %s\n", rt.cfg.Agent.Name, rt.cfg.LLM.Model)
	fmt.Println("Commands: /quit /new /list /load <id> /memory /stats")

	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		rt.shutdown()
		os.Exit(0)
	}()

	var messages []types.Message

	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Commands
		if strings.HasPrefix(input, "/") {
			if handled := rt.handleCommand(input, &messages, &sessID); handled {
				continue
			}
		}

		// Input guard check
		if err := rt.guard.CheckInput(input); err != nil {
			fmt.Printf("⚠ Input rejected: %v\n", err)
			continue
		}

		// Add user message
		userMsg := types.Message{Role: "user", Content: input}
		messages = append(messages, userMsg)

		// Persist user message
		rt.loop.SetPersister(func(msg types.Message) {}) // avoid double-persist
		storedMsg := &store.StoredMessage{
			ID:        generateID(),
			SessionID: sessID,
			Role:      "user",
			Content:   input,
			TokenEst:  ctxmgr.EstimateTokens([]types.Message{userMsg}),
		}
		if err := rt.store.AppendMessage(sessID, storedMsg); err != nil {
			rt.logger.Warn("failed to persist user message", "error", err)
		}

		// Build context with system prompt + memory
		fullMessages := rt.ctxMgr.Build(messages)

		// Run agent loop
		result, err := rt.loop.Run(fullMessages)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		}

		// Strip system message for storage
		messages = rt.ctxMgr.Strip(result)

		// Print last assistant message
		for i := len(result) - 1; i >= 0; i-- {
			if result[i].Role == "assistant" && result[i].Content != "" {
				fmt.Printf("\n%s\n", result[i].Content)
				break
			}
		}

		// Re-enable persister
		rt.loop.SetPersister(func(msg types.Message) {
			storedMsg := &store.StoredMessage{
				ID:        generateID(),
				SessionID: sessID,
				Role:      msg.Role,
				Content:   msg.Content,
				TokenEst:  ctxmgr.EstimateTokens([]types.Message{msg}),
			}
			if len(msg.ToolCalls) > 0 {
				tc, _ := jsonMarshal(msg.ToolCalls)
				storedMsg.ToolCalls = tc
			}
			storedMsg.ToolCallID = msg.ToolCallID
			storedMsg.Name = msg.Name
			if err := rt.store.AppendMessage(sessID, storedMsg); err != nil {
				rt.logger.Warn("failed to persist message", "error", err)
			}
		})

		// Update session
		rt.store.UpdateSession(sessID, map[string]any{
			"message_count": len(messages),
		})
	}

	rt.shutdown()
	return nil
}

func (rt *Runtime) handleCommand(input string, messages *[]types.Message, sessID *string) bool {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/quit", "/exit":
		rt.shutdown()
		os.Exit(0)
	case "/new":
		newID := generateID()
		*sessID = newID
		rt.compactor.WithSession(newID)
		*messages = nil
		sess := &store.SessionMeta{ID: newID, Title: "CLI Session", CreatedAt: time.Now(), UpdatedAt: time.Now()}
		rt.store.CreateSession(sess)
		fmt.Println("New session started.")
	case "/list":
		sessions, _ := rt.store.ListSessions(20)
		for _, s := range sessions {
			fmt.Printf("  %s  %s  msgs:%d\n", s.ID[:8], s.UpdatedAt.Format("15:04"), s.MessageCount)
		}
	case "/load":
		if len(parts) < 2 {
			fmt.Println("Usage: /load <session_id>")
			return true
		}
		loaded, err := rt.store.GetMessages(parts[1], store.MessageOpts{ExcludeCompacted: true})
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return true
		}
		*sessID = parts[1]
		rt.compactor.WithSession(parts[1])
		*messages = nil
		for _, m := range loaded {
			*messages = append(*messages, types.Message{Role: m.Role, Content: m.Content})
		}
		fmt.Printf("Loaded %d messages from session %s\n", len(loaded), parts[1][:8])
	case "/memory":
		text := rt.memory.GetMemoryText()
		if text == "" {
			fmt.Println("No memories stored.")
		} else {
			fmt.Println(text)
		}
	case "/stats":
		count, _ := rt.store.CountMessages(*sessID)
		tokens := ctxmgr.EstimateTokens(*messages)
		fmt.Printf("Session: %s\nMessages: %d\nTokens: ~%d\nBudget: %d\nTools: %d\n",
			(*sessID)[:8], count, tokens, rt.ctxMgr.TokenBudget(), rt.loop.ToolCount.Load())
	default:
		return false
	}
	return true
}

func (rt *Runtime) shutdown() {
	rt.logger.Info("shutting down")
	if rt.memory != nil {
		rt.memory.Shutdown()
	}
	if rt.store != nil {
		rt.store.Close()
	}
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func jsonMarshal(v any) (json.RawMessage, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}
