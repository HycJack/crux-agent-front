package llm

import (
	"fmt"
	"testing"

	"github.com/hermes-go/core/types"
)

// mockProvider for testing
type mockProv struct {
	name  string
	model string
	fail  bool
}

func (m *mockProv) Complete(messages []types.Message, tools []types.ToolSchema) (*Response, error) {
	if m.fail {
		return nil, fmt.Errorf("mock failure: %s", m.name)
	}
	return &Response{
		Message:      types.Message{Role: "assistant", Content: "from " + m.name},
		FinishReason: "stop",
	}, nil
}

func (m *mockProv) Stream(messages []types.Message, tools []types.ToolSchema) (*StreamResponse, error) {
	resp, err := m.Complete(messages, tools)
	if err != nil {
		return nil, err
	}
	ch := make(chan StreamChunk, 2)
	ch <- StreamChunk{Delta: resp.Message.Content}
	ch <- StreamChunk{FinishReason: resp.FinishReason, Done: true}
	return &StreamResponse{Chan: ch, Cancel: func() {}}, nil
}

func (m *mockProv) Model() string { return m.model }

func TestRouterFallback(t *testing.T) {
	r := NewRouter()
	r.Register("primary", &mockProv{"primary", "m1", true})  // fails
	r.Register("backup", &mockProv{"backup", "m2", false})   // works
	r.SetFallbackOrder([]string{"primary", "backup"})

	resp, err := r.Complete([]types.Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("expected fallback to work: %v", err)
	}
	if resp.Message.Content != "from backup" {
		t.Errorf("expected from backup, got %s", resp.Message.Content)
	}
}

func TestRouterAllFail(t *testing.T) {
	r := NewRouter()
	r.Register("a", &mockProv{"a", "m", true})
	r.SetFallbackOrder([]string{"a"})

	_, err := r.Complete([]types.Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestRouterModel(t *testing.T) {
	r := NewRouter()
	r.Register("x", &mockProv{"x", "gpt-4o", false})
	r.SetFallbackOrder([]string{"x"})

	if r.Model() != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %s", r.Model())
	}
}

func TestRetryProvider(t *testing.T) {
	p := &mockProv{"test", "m", false}
	rp := WithRetry(p, 3, 0)
	resp, err := rp.Complete([]types.Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message.Content != "from test" {
		t.Errorf("got %s", resp.Message.Content)
	}
}
