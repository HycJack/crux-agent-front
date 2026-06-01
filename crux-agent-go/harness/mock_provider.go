package harness

import (
	"fmt"
	"sync"

	"github.com/hermes-go/core/llm"
	"github.com/hermes-go/core/types"
)

// MockProvider is a controllable LLM provider for testing.
type MockProvider struct {
	mu        sync.Mutex
	responses []llm.Response
	callCount int
	modelName string
}

func NewMockProvider(responses ...llm.Response) *MockProvider {
	return &MockProvider{
		responses: responses,
		modelName: "mock",
	}
}

func (m *MockProvider) Complete(messages []types.Message, tools []types.ToolSchema) (*llm.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callCount++
	if m.callCount > len(m.responses) {
		return &llm.Response{
			Message:      types.Message{Role: "assistant", Content: "done"},
			FinishReason: "stop",
		}, nil
	}

	resp := m.responses[m.callCount-1]
	return &resp, nil
}

func (m *MockProvider) Model() string { return m.modelName }

func (m *MockProvider) Stream(messages []types.Message, tools []types.ToolSchema) (*llm.StreamResponse, error) {
	resp, err := m.Complete(messages, tools)
	if err != nil {
		return nil, err
	}
	ch := make(chan llm.StreamChunk, 2)
	ch <- llm.StreamChunk{Delta: resp.Message.Content, ToolCalls: resp.Message.ToolCalls}
	ch <- llm.StreamChunk{FinishReason: resp.FinishReason, Done: true}
	return &llm.StreamResponse{Chan: ch, Cancel: func() {}}, nil
}

func (m *MockProvider) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// ToolCallResponse creates a Response that requests a tool call.
func ToolCallResponse(id, name, args string) llm.Response {
	return llm.Response{
		Message: types.Message{
			Role:    "assistant",
			Content: "",
			ToolCalls: []types.ToolCall{
				{ID: id, Type: "function", Function: types.FunctionCall{Name: name, Arguments: args}},
			},
		},
		FinishReason: "tool_calls",
	}
}

// MultiToolCallResponse creates a Response with multiple parallel tool calls.
func MultiToolCallResponse(calls []types.ToolCall) llm.Response {
	return llm.Response{
		Message: types.Message{
			Role:      "assistant",
			ToolCalls: calls,
		},
		FinishReason: "tool_calls",
	}
}

// TextResponse creates a Response with plain text (no tool calls).
func TextResponse(content string) llm.Response {
	return llm.Response{
		Message:      types.Message{Role: "assistant", Content: content},
		FinishReason: "stop",
	}
}

// ErrorResponse creates a Response that simulates an API error.
type ErrorProvider struct {
	Err error
}

func (e *ErrorProvider) Complete(messages []types.Message, tools []types.ToolSchema) (*llm.Response, error) {
	return nil, e.Err
}

func (e *ErrorProvider) Stream(messages []types.Message, tools []types.ToolSchema) (*llm.StreamResponse, error) {
	return nil, e.Err
}

func (e *ErrorProvider) Model() string { return "error-model" }

// RecordingProvider wraps another provider and records all calls.
type RecordingProvider struct {
	inner   llm.Provider
	mu      sync.Mutex
	records []Record
}

type Record struct {
	Messages []types.Message
	Tools    []types.ToolSchema
	Response *llm.Response
	Error    error
}

func NewRecordingProvider(inner llm.Provider) *RecordingProvider {
	return &RecordingProvider{inner: inner}
}

func (r *RecordingProvider) Complete(messages []types.Message, tools []types.ToolSchema) (*llm.Response, error) {
	resp, err := r.inner.Complete(messages, tools)
	r.mu.Lock()
	r.records = append(r.records, Record{Messages: messages, Tools: tools, Response: resp, Error: err})
	r.mu.Unlock()
	return resp, err
}

func (r *RecordingProvider) Model() string { return r.inner.Model() }

func (r *RecordingProvider) Stream(messages []types.Message, tools []types.ToolSchema) (*llm.StreamResponse, error) {
	return r.inner.Stream(messages, tools)
}

func (r *RecordingProvider) Records() []Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Record, len(r.records))
	copy(result, r.records)
	return result
}

// String returns a summary of recorded calls.
func (r *RecordingProvider) String() string {
	records := r.Records()
	s := fmt.Sprintf("RecordingProvider: %d calls\n", len(records))
	for i, rec := range records {
		s += fmt.Sprintf("  [%d] %d messages, %d tools", i, len(rec.Messages), len(rec.Tools))
		if rec.Error != nil {
			s += fmt.Sprintf(", error: %v", rec.Error)
		}
		s += "\n"
	}
	return s
}
