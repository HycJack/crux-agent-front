// Package agent/pipeline provides the message conversion pipeline.
// AgentMessage[] → transformContext() → convertToLlm() → LLM
package agent

import (
	"fmt"
)

// ConvertToLlm converts AgentMessages to standard LLM messages.
// Filters out custom message types (compactionSummary, branchSummary, custom).
func ConvertToLlm(messages []AgentMessage) []AgentMessage {
	var result []AgentMessage
	for _, msg := range messages {
		switch msg.Role {
		case "user", "assistant", "tool", "system":
			result = append(result, msg)
		case "compactionSummary":
			result = append(result, AgentMessage{
				Role:    "user",
				Content: formatCompactionSummary(msg),
			})
		case "branchSummary":
			result = append(result, AgentMessage{
				Role:    "user",
				Content: formatBranchSummary(msg),
			})
		case "custom":
			if msg.Display {
				result = append(result, AgentMessage{
					Role:    "user",
					Content: msg.Content,
				})
			}
		}
	}
	return result
}

// TransformContext applies context transformations: compression, injection, etc.
type ContextTransform func(messages []AgentMessage) []AgentMessage

// ApplyTransforms applies a chain of context transforms.
func ApplyTransforms(messages []AgentMessage, transforms ...ContextTransform) []AgentMessage {
	result := messages
	for _, t := range transforms {
		if t != nil {
			result = t(result)
		}
	}
	return result
}

// InjectMemory adds memory context after the system message.
func InjectMemory(memoryText string) ContextTransform {
	return func(messages []AgentMessage) []AgentMessage {
		if memoryText == "" {
			return messages
		}
		result := make([]AgentMessage, 0, len(messages)+1)
		if len(messages) > 0 && messages[0].Role == "system" {
			result = append(result, messages[0])
			result = append(result, AgentMessage{
				Role:    "user",
				Content: "[Memory Context]\n" + memoryText,
			})
			result = append(result, messages[1:]...)
		} else {
			result = append(result, AgentMessage{
				Role:    "user",
				Content: "[Memory Context]\n" + memoryText,
			})
			result = append(result, messages...)
		}
		return result
	}
}

// InjectSkills adds skill descriptions after the system message.
func InjectSkills(skillDesc string) ContextTransform {
	return func(messages []AgentMessage) []AgentMessage {
		if skillDesc == "" {
			return messages
		}
		result := make([]AgentMessage, 0, len(messages)+1)
		if len(messages) > 0 && messages[0].Role == "system" {
			result = append(result, messages[0])
			result = append(result, AgentMessage{
				Role:    "user",
				Content: skillDesc,
			})
			result = append(result, messages[1:]...)
		} else {
			result = append(result, AgentMessage{
				Role:    "user",
				Content: skillDesc,
			})
			result = append(result, messages...)
		}
		return result
	}
}

// TrimToBudget removes old messages to fit within a token budget.
func TrimToBudget(maxTokens int, estimator func([]AgentMessage) int) ContextTransform {
	return func(messages []AgentMessage) []AgentMessage {
		if estimator == nil {
			return messages
		}
		total := estimator(messages)
		if total <= maxTokens {
			return messages
		}
		for len(messages) > 2 {
			if messages[0].Role == "assistant" && len(messages[0].ToolCalls) > 0 {
				skip := 1
				for skip < len(messages) && messages[skip].Role == "tool" {
					skip++
				}
				messages = messages[skip:]
			} else {
				messages = messages[1:]
			}
			total = estimator(messages)
			if total <= maxTokens {
				break
			}
		}
		return messages
	}
}

func formatCompactionSummary(msg AgentMessage) string {
	return fmt.Sprintf(
		"The conversation history before this point was compacted into the following summary:\n\n<summary>\n%s\n</summary>\n\n(Tokens before compaction: %d)",
		msg.Summary, msg.TokensBefore,
	)
}

func formatBranchSummary(msg AgentMessage) string {
	return fmt.Sprintf(
		"The following is a summary of a branch that this conversation came back from:\n\n<summary>\n%s\n</summary>",
		msg.Summary,
	)
}

// CreateBranchSummaryMessage creates a branch summary message.
func CreateBranchSummaryMessage(summary, fromID string, timestamp int64) AgentMessage {
	return AgentMessage{
		Role:      "branchSummary",
		Summary:   summary,
		FromID:    fromID,
		Timestamp: timestamp,
	}
}

// CreateCompactionSummaryMessage creates a compaction summary message.
func CreateCompactionSummaryMessage(summary string, tokensBefore int, timestamp int64) AgentMessage {
	return AgentMessage{
		Role:         "compactionSummary",
		Summary:      summary,
		TokensBefore: tokensBefore,
		Timestamp:    timestamp,
	}
}

// CreateCustomMessage creates a custom message.
func CreateCustomMessage(customType, content string, display bool, timestamp int64) AgentMessage {
	return AgentMessage{
		Role:       "custom",
		CustomType: customType,
		Content:    content,
		Display:    display,
		Timestamp:  timestamp,
	}
}
