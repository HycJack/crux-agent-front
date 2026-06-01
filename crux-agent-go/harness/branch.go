package harness

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hermes-go/core/agent"
)

// BranchSummarizer generates summaries when switching branches.
// Inspired by pi's branch-summarization.ts.
type BranchSummarizer struct {
	streamFn func(ctx context.Context, messages []agent.AgentMessage, tools []agent.AgentTool) (*agent.StreamResult, error)
}

func NewBranchSummarizer(streamFn func(ctx context.Context, messages []agent.AgentMessage, tools []agent.AgentTool) (*agent.StreamResult, error)) *BranchSummarizer {
	return &BranchSummarizer{streamFn: streamFn}
}

// SummarizeBranch generates a summary of the messages in a branch.
func (bs *BranchSummarizer) SummarizeBranch(messages []agent.AgentMessage) (string, error) {
	if bs.streamFn == nil {
		return bs.generateTruncateSummary(messages), nil
	}

	// Build summary prompt
	var sb strings.Builder
	sb.WriteString("Summarize the following conversation branch concisely.\n")
	sb.WriteString("This branch will be left, so preserve key context that might be useful when returning.\n\n")
	sb.WriteString("Branch conversation:\n")
	for _, msg := range messages {
		content := msg.Content
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, content))
	}

	summaryMessages := []agent.AgentMessage{
		{Role: "user", Content: sb.String()},
	}

	result, err := bs.streamFn(context.Background(), summaryMessages, nil)
	if err != nil {
		return bs.generateTruncateSummary(messages), err
	}

	return result.Message.Content, nil
}

func (bs *BranchSummarizer) generateTruncateSummary(messages []agent.AgentMessage) string {
	var sb strings.Builder
	sb.WriteString("Branch summary (truncated):\n")
	count := 0
	for _, msg := range messages {
		if msg.Role == "user" || msg.Role == "assistant" {
			content := msg.Content
			if len(content) > 100 {
				content = content[:100] + "..."
			}
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", msg.Role, content))
			count++
			if count >= 5 {
				sb.WriteString("- ... (more messages omitted)\n")
				break
			}
		}
	}
	return sb.String()
}

// CollectEntriesForBranchSummary collects messages from a branch for summarization.
func CollectEntriesForBranchSummary(entries []SessionTreeEntry, fromID string) []agent.AgentMessage {
	var messages []agent.AgentMessage
	collecting := false
	for _, entry := range entries {
		if entry.ID == fromID {
			collecting = true
		}
		if collecting && entry.Message != nil {
			messages = append(messages, *entry.Message)
		}
	}
	return messages
}

// GenerateBranchSummary creates a branch summary entry.
func GenerateBranchSummary(session *Session, fromID string, summarizer *BranchSummarizer) error {
	entries, err := session.Entries()
	if err != nil {
		return err
	}

	messages := CollectEntriesForBranchSummary(entries, fromID)
	if len(messages) == 0 {
		return nil
	}

	summary, err := summarizer.SummarizeBranch(messages)
	if err != nil {
		return err
	}

	entry := SessionTreeEntry{
		ID:        generateID(),
		Type:      "branch_summary",
		Summary:   summary,
		FromID:    fromID,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	return session.storage.AppendEntries([]SessionTreeEntry{entry})
}
