package compaction

import (
	"fmt"
	"strings"

	ctxmgr "github.com/hermes-go/core/context"
	"github.com/hermes-go/core/llm"
	"github.com/hermes-go/core/store"
	"github.com/hermes-go/core/types"
)

// Strategy defines how compaction works.
type Strategy string

const (
	StrategyTruncate  Strategy = "truncate"
	StrategySummarize Strategy = "summarize"
	StrategyTwoStage  Strategy = "two_stage"
)

// Compactor handles context compression.
type Compactor struct {
	provider   llm.Provider
	store      store.Store
	sessionID  string
	strategy   Strategy
	keepRecent int
	maxTokens  int
}

func New(provider llm.Provider) *Compactor {
	return &Compactor{
		provider:   provider,
		strategy:   StrategyTwoStage,
		keepRecent: 10,
		maxTokens:  4000,
	}
}

func (c *Compactor) WithStrategy(s Strategy) *Compactor  { c.strategy = s; return c }
func (c *Compactor) WithKeepRecent(n int) *Compactor     { c.keepRecent = n; return c }
func (c *Compactor) WithMaxTokens(n int) *Compactor      { c.maxTokens = n; return c }
func (c *Compactor) WithStore(s store.Store) *Compactor   { c.store = s; return c }
func (c *Compactor) WithSession(id string) *Compactor     { c.sessionID = id; return c }

// CompactForLoop returns a function compatible with agentloop.Loop.SetCompactor.
func (c *Compactor) CompactForLoop() func([]types.Message) []types.Message {
	return func(messages []types.Message) []types.Message {
		totalTokens := ctxmgr.EstimateTokens(messages)
		// Compact if we exceed 80% of typical context window
		if totalTokens < 100000 {
			return messages
		}

		result, didCompact := c.Compact(messages, 80000)
		if didCompact && c.store != nil && c.sessionID != "" {
			// Mark old messages as compacted in store
			// We mark all messages before the first kept message
			c.store.MarkCompacted(c.sessionID, "compacted")
		}
		return result
	}
}

// Compact compresses messages when they exceed the token budget.
func (c *Compactor) Compact(messages []types.Message, tokenBudget int) ([]types.Message, bool) {
	totalTokens := ctxmgr.EstimateTokens(messages)
	if totalTokens <= tokenBudget {
		return messages, false
	}

	switch c.strategy {
	case StrategyTruncate:
		return c.truncate(messages), true
	case StrategySummarize:
		return c.summarize(messages), true
	case StrategyTwoStage:
		return c.twoStage(messages), true
	default:
		return c.truncate(messages), true
	}
}

func (c *Compactor) truncate(messages []types.Message) []types.Message {
	if len(messages) <= c.keepRecent {
		return messages
	}
	// Keep first message (system context) + last N messages
	result := make([]types.Message, 0, c.keepRecent+1)
	result = append(result, messages[0])
	result = append(result, messages[len(messages)-c.keepRecent:]...)
	return result
}

func (c *Compactor) summarize(messages []types.Message) []types.Message {
	if len(messages) <= c.keepRecent || c.provider == nil {
		return c.truncate(messages)
	}

	// Messages to summarize (everything except first and last N)
	toSummarize := messages[1 : len(messages)-c.keepRecent]
	recent := messages[len(messages)-c.keepRecent:]

	// Build summary prompt
	var sb strings.Builder
	sb.WriteString("Summarize the following conversation concisely, preserving key facts, decisions, and context:\n\n")
	for _, msg := range toSummarize {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, truncateStr(msg.Content, 200)))
	}

	summaryMsg := []types.Message{
		{Role: "user", Content: sb.String()},
	}

	resp, err := c.provider.Complete(summaryMsg, nil)
	if err != nil {
		return c.truncate(messages)
	}

	result := make([]types.Message, 0, c.keepRecent+2)
	result = append(result, messages[0])
	result = append(result, types.Message{
		Role:    "user",
		Content: "[Conversation Summary]\n" + resp.Message.Content,
	})
	result = append(result, recent...)
	return result
}

func (c *Compactor) twoStage(messages []types.Message) []types.Message {
	// Stage 1: Remove old tool results
	stage1 := c.removeOldToolResults(messages)

	// Check if still over budget
	if ctxmgr.EstimateTokens(stage1) <= c.maxTokens {
		return stage1
	}

	// Stage 2: Summarize remaining old messages
	return c.summarize(stage1)
}

func (c *Compactor) removeOldToolResults(messages []types.Message) []types.Message {
	if len(messages) <= c.keepRecent {
		return messages
	}

	// Find the boundary: keep recent messages intact
	keepFrom := len(messages) - c.keepRecent
	if keepFrom < 1 {
		keepFrom = 1
	}

	result := make([]types.Message, 0, len(messages))
	for i, msg := range messages {
		if i >= keepFrom {
			// Keep recent messages as-is
			result = append(result, msg)
		} else if msg.Role == "tool" {
			// Replace old tool results with a placeholder
			result = append(result, types.Message{
				Role:    "tool",
				Content: "[result compacted]",
				Name:    msg.Name,
			})
		} else {
			result = append(result, msg)
		}
	}
	return result
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
