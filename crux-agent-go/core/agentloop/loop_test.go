package agentloop

import (
	"fmt"
	"testing"

	"github.com/hermes-go/core/llm"
	"github.com/hermes-go/core/types"
)

// mockProvider returns pre-defined responses.
type mockProvider struct {
	responses []*llm.Response
	callCount int
}

func (m *mockProvider) Complete(messages []types.Message, tools []types.ToolSchema) (*llm.Response, error) {
	if m.callCount >= len(m.responses) {
		return nil, fmt.Errorf("no more mock responses")
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

func (m *mockProvider) Model() string { return "mock-model" }

func (m *mockProvider) Stream(messages []types.Message, tools []types.ToolSchema) (*llm.StreamResponse, error) {
	resp, err := m.Complete(messages, tools)
	if err != nil {
		return nil, err
	}
	ch := make(chan llm.StreamChunk, 2)
	ch <- llm.StreamChunk{Delta: resp.Message.Content, ToolCalls: resp.Message.ToolCalls}
	ch <- llm.StreamChunk{FinishReason: resp.FinishReason, Done: true}
	return &llm.StreamResponse{Chan: ch, Cancel: func() {}}, nil
}

func TestLoopSimpleReply(t *testing.T) {
	provider := &mockProvider{
		responses: []*llm.Response{
			{
				Message: types.Message{Role: "assistant", Content: "Hello!"},
				FinishReason: "stop",
			},
		},
	}

	loop := New(provider, nil, nil)
	loop.MaxRounds = 5

	messages := []types.Message{
		{Role: "user", Content: "Hi"},
	}

	result, err := loop.Run(messages)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// Should have user + assistant
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[1].Content != "Hello!" {
		t.Errorf("response: got %s", result[1].Content)
	}
	if provider.callCount != 1 {
		t.Errorf("expected 1 LLM call, got %d", provider.callCount)
	}
}

func TestLoopToolCall(t *testing.T) {
	provider := &mockProvider{
		responses: []*llm.Response{
			// First call: LLM wants to call a tool
			{
				Message: types.Message{
					Role: "assistant",
					ToolCalls: []types.ToolCall{
						{
							ID:   "call-1",
							Type: "function",
							Function: types.FunctionCall{
								Name:      "get_time",
								Arguments: "{}",
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
			// Second call: LLM sees tool result and replies
			{
				Message: types.Message{
					Role:    "assistant",
					Content: "The current time is 2026-05-30.",
				},
				FinishReason: "stop",
			},
		},
	}

	executor := func(call types.ToolCall) types.ToolResult {
		if call.Function.Name != "get_time" {
			t.Errorf("unexpected tool: %s", call.Function.Name)
		}
		return types.ToolResult{
			ToolCallID: call.ID,
			Content:    "UTC: 2026-05-30T12:00:00Z",
		}
	}

	tools := []types.ToolSchema{
		{Name: "get_time", Description: "Get current time"},
	}

	loop := New(provider, tools, executor)
	loop.MaxRounds = 5

	messages := []types.Message{
		{Role: "user", Content: "What time is it?"},
	}

	result, err := loop.Run(messages)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// user + assistant(tool_call) + tool_result + assistant(reply)
	if len(result) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(result))
	}
	if result[2].Role != "tool" {
		t.Errorf("third message should be tool, got %s", result[2].Role)
	}
	if result[3].Content != "The current time is 2026-05-30." {
		t.Errorf("final reply: got %s", result[3].Content)
	}
}

func TestLoopMaxRounds(t *testing.T) {
	// LLM keeps calling tools forever
	responses := make([]*llm.Response, 25)
	for i := range responses {
		responses[i] = &llm.Response{
			Message: types.Message{
				Role: "assistant",
				ToolCalls: []types.ToolCall{
					{ID: fmt.Sprintf("call-%d", i), Type: "function",
						Function: types.FunctionCall{Name: "noop", Arguments: "{}"}},
				},
			},
			FinishReason: "tool_calls",
		}
	}

	provider := &mockProvider{responses: responses}
	executor := func(call types.ToolCall) types.ToolResult {
		return types.ToolResult{ToolCallID: call.ID, Content: "ok"}
	}

	loop := New(provider, nil, executor)
	loop.MaxRounds = 3 // Limit to 3

	_, err := loop.Run([]types.Message{{Role: "user", Content: "loop forever"}})
	if err == nil {
		t.Fatal("expected error for max rounds exceeded")
	}
}
