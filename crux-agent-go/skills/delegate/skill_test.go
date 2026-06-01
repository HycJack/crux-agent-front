package delegate

import (
	"testing"

	"github.com/hermes-go/core/registry"
	"github.com/hermes-go/core/transport"
	"github.com/hermes-go/core/types"
)

func TestDelegateDiscoverEmpty(t *testing.T) {
	reg := registry.New()
	s := New(reg)

	result := s.Handle(types.ToolCall{
		ID:       "c1",
		Function: types.FunctionCall{Name: "discover_agents", Arguments: "{}"},
	})
	if result.Content != "No agents available." {
		t.Errorf("expected no agents, got: %s", result.Content)
	}
}

func TestDelegateDiscoverWithAgents(t *testing.T) {
	reg := registry.New()
	reg.Register(&transport.AgentCard{
		Name:        "coder",
		Description: "Writes code",
		Skills:      []string{"terminal"},
	}, nil, "memory:")

	s := New(reg)
	result := s.Handle(types.ToolCall{
		ID:       "c1",
		Function: types.FunctionCall{Name: "discover_agents", Arguments: "{}"},
	})
	if result.IsError {
		t.Fatalf("error: %s", result.Content)
	}
	if result.Content == "No agents available." {
		t.Error("should find coder agent")
	}
}

func TestDelegateTaskNoAgent(t *testing.T) {
	reg := registry.New()
	s := New(reg)

	result := s.Handle(types.ToolCall{
		ID: "c1",
		Function: types.FunctionCall{
			Name:      "delegate_task",
			Arguments: `{"agent":"nonexistent","task":"do something"}`,
		},
	})
	if !result.IsError {
		t.Fatal("expected error for missing agent")
	}
}

func TestDelegateTaskSuccess(t *testing.T) {
	reg := registry.New()
	handler := func(task *transport.Task) (*transport.TaskResult, error) {
		return &transport.TaskResult{
			Status:   "completed",
			Messages: []types.Message{{Role: "assistant", Content: "task done"}},
		}, nil
	}
	tr := transport.NewInProcess(handler)
	reg.Register(&transport.AgentCard{
		Name:   "worker",
		Skills: []string{"terminal"},
	}, tr, "memory:")

	s := New(reg)
	result := s.Handle(types.ToolCall{
		ID: "c1",
		Function: types.FunctionCall{
			Name:      "delegate_task",
			Arguments: `{"agent":"worker","task":"run tests"}`,
		},
	})
	if result.IsError {
		t.Fatalf("error: %s", result.Content)
	}
	if result.Content != "task done\n" {
		t.Errorf("got: %s", result.Content)
	}
}
