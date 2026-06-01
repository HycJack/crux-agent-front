package harness

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/hermes-go/core/agent"
)

type CompactionSettings struct {
	Enabled          bool `yaml:"enabled"`
	ReserveTokens    int  `yaml:"reserve_tokens"`
	KeepRecentTokens int  `yaml:"keep_recent_tokens"`
}

var DefaultCompactionSettings = CompactionSettings{
	Enabled:          true,
	ReserveTokens:    16384,
	KeepRecentTokens: 20000,
}

type CompactionResult struct {
	Summary          string
	FirstKeptEntryID string
	TokensBefore     int
	TokensAfter      int
}

type Compactor struct {
	settings CompactionSettings
	streamFn func(ctx context.Context, messages []agent.AgentMessage, tools []agent.AgentTool) (*agent.StreamResult, error)
}

func NewCompactor(settings CompactionSettings, streamFn func(ctx context.Context, messages []agent.AgentMessage, tools []agent.AgentTool) (*agent.StreamResult, error)) *Compactor {
	return &Compactor{settings: settings, streamFn: streamFn}
}

func (c *Compactor) ShouldCompact(messages []agent.AgentMessage) bool {
	if !c.settings.Enabled {
		return false
	}
	totalTokens := estimateTokens(messages)
	threshold := 128000 - c.settings.ReserveTokens - c.settings.KeepRecentTokens
	return totalTokens > threshold
}

func (c *Compactor) Compact(messages []agent.AgentMessage, entries []SessionTreeEntry) (*CompactionResult, error) {
	totalTokens := estimateTokens(messages)

	// Find the split point: keep recent messages verbatim
	keepTokens := 0
	splitIdx := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := estimateTokens([]agent.AgentMessage{messages[i]})
		if keepTokens+msgTokens > c.settings.KeepRecentTokens {
			break
		}
		keepTokens += msgTokens
		splitIdx = i
	}

	// FIX: handle edge cases
	if splitIdx == 0 {
		return nil, nil // Nothing to compact
	}
	if splitIdx >= len(messages) {
		// All messages would be compacted — keep at least the last message
		splitIdx = len(messages) - 1
		if splitIdx == 0 {
			return nil, nil
		}
	}

	toCompact := messages[:splitIdx]
	kept := messages[splitIdx:]

	summary, err := c.generateSummary(toCompact)
	if err != nil {
		summary = generateTruncateSummary(toCompact)
	}

	// FIX: find first kept entry ID by matching message role + content prefix
	// (more robust than exact content match)
	firstKeptID := ""
	if len(kept) > 0 {
		for _, entry := range entries {
			if entry.Message != nil &&
				entry.Message.Role == kept[0].Role &&
				len(entry.Message.Content) > 0 &&
				len(kept[0].Content) > 0 &&
				truncEqual(entry.Message.Content, kept[0].Content, 50) {
				firstKeptID = entry.ID
				break
			}
		}
	}

	return &CompactionResult{
		Summary:          summary,
		FirstKeptEntryID: firstKeptID,
		TokensBefore:     totalTokens,
		TokensAfter:      estimateTokens(kept) + estimateTokens([]agent.AgentMessage{{Content: summary}}),
	}, nil
}

// truncEqual compares the first n runes of two strings.
func truncEqual(a, b string, n int) bool {
	ra := []rune(a)
	rb := []rune(b)
	if len(ra) > n {
		ra = ra[:n]
	}
	if len(rb) > n {
		rb = rb[:n]
	}
	return string(ra) == string(rb)
}

func (c *Compactor) generateSummary(messages []agent.AgentMessage) (string, error) {
	if c.streamFn == nil {
		return generateTruncateSummary(messages), nil
	}

	var sb strings.Builder
	sb.WriteString("Summarize the following conversation concisely. Preserve:\n")
	sb.WriteString("- Key decisions and their rationale\n")
	sb.WriteString("- File paths that were read or modified\n")
	sb.WriteString("- Important facts, names, and numbers\n")
	sb.WriteString("- Current task state and next steps\n\n")
	sb.WriteString("Conversation:\n")
	for _, msg := range messages {
		content := msg.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, content))
	}

	summaryMessages := []agent.AgentMessage{
		{Role: "user", Content: sb.String()},
	}

	result, err := c.streamFn(context.Background(), summaryMessages, nil)
	if err != nil {
		return generateTruncateSummary(messages), err
	}

	return result.Message.Content, nil
}

func generateTruncateSummary(messages []agent.AgentMessage) string {
	var sb strings.Builder
	sb.WriteString("Previous conversation summary (truncated):\n")
	count := 0
	for _, msg := range messages {
		if msg.Role == "user" || msg.Role == "assistant" {
			content := msg.Content
			if len(content) > 100 {
				content = content[:100] + "..."
			}
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", msg.Role, content))
			count++
			if count >= 10 {
				sb.WriteString("- ... (more messages omitted)\n")
				break
			}
		}
	}
	return sb.String()
}

func estimateTokens(messages []agent.AgentMessage) int {
	total := 0
	for _, msg := range messages {
		total += utf8.RuneCountInString(msg.Content) / 2
		total += utf8.RuneCountInString(msg.Summary) / 2
		for _, tc := range msg.ToolCalls {
			total += utf8.RuneCountInString(tc.Function.Name) / 2
			total += utf8.RuneCountInString(tc.Function.Arguments) / 2
		}
	}
	if total == 0 {
		total = 1
	}
	return total
}
