package agentloop

import (
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/hermes-go/core/llm"
	"github.com/hermes-go/core/types"
)

// ToolExecutor runs a tool call and returns the result.
type ToolExecutor func(call types.ToolCall) types.ToolResult

// MessagePersister saves a single message to persistent storage.
type MessagePersister func(msg types.Message)

// Compactor checks token budget and compacts if needed.
type Compactor func(messages []types.Message) []types.Message

// InputGuard checks user input before processing.
type InputGuard func(input string) error

// OutputGuard checks tool output before returning to LLM.
type OutputGuard func(name string, result types.ToolResult) types.ToolResult

// Loop is the core reasoning cycle.
type Loop struct {
	provider     llm.Provider
	tools        []types.ToolSchema
	executor     ToolExecutor
	persister    MessagePersister
	compactor    Compactor
	inputGuard   InputGuard
	outputGuard  OutputGuard
	logger       *slog.Logger
	MaxRounds    int
	ToolCount    atomic.Int64
}

func New(provider llm.Provider, tools []types.ToolSchema, executor ToolExecutor) *Loop {
	return &Loop{
		provider:  provider,
		tools:     tools,
		executor:  executor,
		logger:    slog.Default(),
		MaxRounds: 20,
	}
}

// SetPersister sets the message persistence callback.
func (l *Loop) SetPersister(fn MessagePersister) { l.persister = fn }

// SetCompactor sets the context compaction callback.
func (l *Loop) SetCompactor(fn Compactor) { l.compactor = fn }

// SetInputGuard sets the input guardrail callback.
func (l *Loop) SetInputGuard(fn InputGuard) { l.inputGuard = fn }

// SetOutputGuard sets the output guardrail callback.
func (l *Loop) SetOutputGuard(fn OutputGuard) { l.outputGuard = fn }

// persist saves a message if persister is set.
func (l *Loop) persist(msg types.Message) {
	if l.persister != nil {
		l.persister(msg)
	}
}

// Run executes the agent loop until the LLM stops calling tools.
func (l *Loop) Run(messages []types.Message) ([]types.Message, error) {
	for round := 0; round < l.MaxRounds; round++ {
		l.logger.Debug("agent loop", "round", round, "messages", len(messages))

		// Apply compaction if set
		if l.compactor != nil {
			messages = l.compactor(messages)
		}

		// Call LLM
		resp, err := l.provider.Complete(messages, l.tools)
		if err != nil {
			return messages, fmt.Errorf("llm error at round %d: %w", round, err)
		}

		// Append and persist assistant message
		messages = append(messages, resp.Message)
		l.persist(resp.Message)

		// No tool calls → done
		if len(resp.Message.ToolCalls) == 0 {
			l.logger.Debug("loop done", "reason", resp.FinishReason, "round", round)
			return messages, nil
		}

		// Execute each tool call
		for _, call := range resp.Message.ToolCalls {
			l.logger.Debug("executing tool", "tool", call.Function.Name, "id", call.ID)
			l.ToolCount.Add(1)

			result := l.executor(call)

			// Apply output guard
			if l.outputGuard != nil {
				result = l.outputGuard(call.Function.Name, result)
			}

			toolMsg := types.Message{
				Role:       "tool",
				ToolCallID: result.ToolCallID,
				Content:    result.Content,
				Name:       call.Function.Name,
			}
			messages = append(messages, toolMsg)
			l.persist(toolMsg)
		}
	}

	return messages, fmt.Errorf("max rounds (%d) exceeded", l.MaxRounds)
}
