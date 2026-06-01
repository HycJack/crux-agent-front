package harness

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/hermes-go/core/agent"
)

type AgentHarnessOptions struct {
	SystemPrompt       string
	SkillDirs          []string
	SessionPath        string
	CompactionSettings CompactionSettings
	Tools              []agent.AgentTool
	StreamFn           func(ctx context.Context, messages []agent.AgentMessage, tools []agent.AgentTool) (*agent.StreamResult, error)
	BeforeToolCall     func(ctx agent.BeforeToolCallContext) *agent.BeforeToolCallResult
	AfterToolCall      func(ctx agent.AfterToolCallContext) *agent.AfterToolCallResult
	OnEvent            func(event agent.AgentEvent)
}

type AgentHarness struct {
	agent        *agent.Agent
	session      *Session
	compactor    *Compactor
	skills       []Skill
	systemPrompt string
	options      AgentHarnessOptions
}

func NewAgentHarness(opts AgentHarnessOptions) (*AgentHarness, error) {
	var skills []Skill
	if len(opts.SkillDirs) > 0 {
		loaded, warnings := LoadSkills(opts.SkillDirs...)
		skills = loaded
		for _, w := range warnings {
			fmt.Printf("Warning: %s\n", w)
		}
	}

	systemPrompt := opts.SystemPrompt
	if len(skills) > 0 {
		skillBlock := FormatSkillsForSystemPrompt(skills)
		if skillBlock != "" {
			systemPrompt += "\n\n" + skillBlock
		}
	}

	var storage SessionStorage
	if opts.SessionPath != "" {
		storage = NewJSONLStorage(opts.SessionPath)
	} else {
		storage = NewMemoryStorage()
	}
	session := NewSession(storage)

	compactor := NewCompactor(opts.CompactionSettings, opts.StreamFn)

	agentConfig := agent.AgentLoopConfig{
		Tools:          opts.Tools,
		StreamFn:       opts.StreamFn,
		MaxRounds:      50,
		BeforeToolCall: opts.BeforeToolCall,
		AfterToolCall:  opts.AfterToolCall,
		OnEvent:        opts.OnEvent,
	}
	a := agent.New(agentConfig)

	return &AgentHarness{
		agent:        a,
		session:      session,
		compactor:    compactor,
		skills:       skills,
		systemPrompt: systemPrompt,
		options:      opts,
	}, nil
}

func (h *AgentHarness) Prompt(text string) ([]agent.AgentMessage, error) {
	messages, err := h.session.BuildContext()
	if err != nil {
		return nil, fmt.Errorf("build context: %w", err)
	}

	// Check if compaction is needed
	if h.compactor.ShouldCompact(messages) {
		entries, err := h.session.Entries()
		if err != nil {
			return nil, fmt.Errorf("get entries: %w", err)
		}
		compactResult, err := h.compactor.Compact(messages, entries)
		if err != nil {
			return nil, fmt.Errorf("compact: %w", err)
		}
		if compactResult != nil {
			compEntry := SessionTreeEntry{
				ID:               generateID(),
				Type:             "compaction",
				Summary:          compactResult.Summary,
				TokensBefore:     compactResult.TokensBefore,
				FirstKeptEntryID: compactResult.FirstKeptEntryID,
				Timestamp:        time.Now().Format(time.RFC3339),
			}
			if err := h.session.storage.AppendEntries([]SessionTreeEntry{compEntry}); err != nil {
				return nil, fmt.Errorf("append compaction: %w", err)
			}
			messages, err = h.session.BuildContext()
			if err != nil {
				return nil, fmt.Errorf("rebuild context: %w", err)
			}
		}
	}

	// Build full message list
	fullMessages := make([]agent.AgentMessage, 0, len(messages)+2)
	fullMessages = append(fullMessages, agent.AgentMessage{
		Role:      "system",
		Content:   h.systemPrompt,
		Timestamp: time.Now().UnixMilli(),
	})
	fullMessages = append(fullMessages, messages...)

	userMsg := agent.AgentMessage{
		Role:      "user",
		Content:   text,
		Timestamp: time.Now().UnixMilli(),
	}

	// Persist user message ONCE
	if err := h.session.AppendMessage(userMsg); err != nil {
		return nil, fmt.Errorf("persist user message: %w", err)
	}

	// Run agent
	result, err := h.agent.Run(append(fullMessages, userMsg))
	if err != nil {
		return nil, err
	}

	// Persist assistant messages (skip system and user which are already persisted)
	for _, msg := range result {
		if msg.Role == "assistant" {
			if err := h.session.AppendMessage(msg); err != nil {
				return nil, fmt.Errorf("persist assistant message: %w", err)
			}
		}
	}

	return result, nil
}

func (h *AgentHarness) Session() *Session        { return h.session }
func (h *AgentHarness) Skills() []Skill           { return h.skills }
func (h *AgentHarness) Agent() *agent.Agent        { return h.agent }
func (h *AgentHarness) Close() error              { return h.session.storage.Close() }

func (h *AgentHarness) Fork() (*AgentHarness, error) {
	newSession, err := h.session.Fork("")
	if err != nil {
		return nil, err
	}
	newOpts := h.options
	return &AgentHarness{
		agent:        h.agent,
		session:      newSession,
		compactor:    h.compactor,
		skills:       h.skills,
		systemPrompt: h.systemPrompt,
		options:      newOpts,
	}, nil
}

func SessionPath(sessionDir, sessionID string) string {
	return filepath.Join(sessionDir, sessionID+".jsonl")
}
