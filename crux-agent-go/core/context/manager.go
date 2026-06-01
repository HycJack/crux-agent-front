package context

import (
	"strings"
	"unicode/utf8"

	"github.com/hermes-go/core/types"
)

// Manager builds the prompt sent to the LLM with token budget control.
type Manager struct {
	systemPrompt string
	memoryText   string
	skillDescs   []string
	maxTokens    int
}

func New(systemPrompt string) *Manager {
	return &Manager{systemPrompt: systemPrompt, maxTokens: 128000}
}

func NewWithBudget(systemPrompt string, maxTokens int) *Manager {
	return &Manager{systemPrompt: systemPrompt, maxTokens: maxTokens}
}

// SetMemory sets the memory content to inject into system prompt.
func (m *Manager) SetMemory(text string) { m.memoryText = text }

// SetSkillDescriptions sets skill descriptions to inject.
func (m *Manager) SetSkillDescriptions(descs []string) { m.skillDescs = descs }

// Build prepends the system prompt (with memory and skill descs) to the message list.
func (m *Manager) Build(messages []types.Message) []types.Message {
	result := make([]types.Message, 0, len(messages)+1)

	// Compose system prompt with memory and skill descriptions
	sb := strings.Builder{}
	sb.WriteString(m.systemPrompt)

	if m.memoryText != "" {
		sb.WriteString("\n\n[Memory Context]\n")
		sb.WriteString(m.memoryText)
	}

	if len(m.skillDescs) > 0 {
		sb.WriteString("\n\n[Available Skills]\n")
		for _, desc := range m.skillDescs {
			sb.WriteString("- ")
			sb.WriteString(desc)
			sb.WriteString("\n")
		}
	}

	result = append(result, types.Message{
		Role:    "system",
		Content: sb.String(),
	})
	result = append(result, messages...)
	return result
}

// Strip removes the system message from the front (for storage).
func (m *Manager) Strip(messages []types.Message) []types.Message {
	if len(messages) > 0 && messages[0].Role == "system" {
		return messages[1:]
	}
	return messages
}

// Trim removes old messages to fit within the token budget.
// Preserves tool call/result pairs.
func (m *Manager) Trim(messages []types.Message) []types.Message {
	total := EstimateTokens(messages)
	if total <= m.maxTokens {
		return messages
	}

	// Drop oldest messages one at a time, but never split a tool call/result pair
	for len(messages) > 2 {
		// Don't drop if the next message is a tool result for this message
		if messages[0].Role == "assistant" && len(messages[0].ToolCalls) > 0 {
			// Skip past the tool result messages
			skip := 1
			for skip < len(messages) && messages[skip].Role == "tool" {
				skip++
			}
			messages = messages[skip:]
		} else {
			messages = messages[1:]
		}

		total = EstimateTokens(messages)
		if total <= m.maxTokens {
			break
		}
	}

	return messages
}

// InjectMemory adds memory context after the system message (legacy, now integrated into Build).
func (m *Manager) InjectMemory(messages []types.Message, memoryContent string) []types.Message {
	if memoryContent == "" {
		return messages
	}
	result := make([]types.Message, 0, len(messages)+1)
	result = append(result, messages[0]) // system
	result = append(result, types.Message{
		Role:    "user",
		Content: "[Memory Context]\n" + memoryContent,
	})
	result = append(result, messages[1:]...)
	return result
}

// TokenBudget returns the max tokens.
func (m *Manager) TokenBudget() int { return m.maxTokens }

// EstimateTokens estimates token count for messages using rune count.
// Approximation: 1 token ≈ 2 runes (better for CJK than char/4).
func EstimateTokens(messages []types.Message) int {
	total := 0
	for _, msg := range messages {
		total += utf8.RuneCountInString(msg.Content) / 2
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
