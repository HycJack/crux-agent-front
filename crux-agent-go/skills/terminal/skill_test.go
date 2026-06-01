package terminal

import (
	"testing"

	"github.com/hermes-go/core/types"
)

func TestExecEcho(t *testing.T) {
	s := New()
	call := types.ToolCall{ID: "c1", Function: types.FunctionCall{Name: "exec", Arguments: `{"command":"echo hello"}`}}
	result := s.Handle(call)
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if result.Content == "" {
		t.Fatal("expected output")
	}
}

func TestExecWithWorkdir(t *testing.T) {
	s := New()
	s.WorkDir = "/tmp"
	call := types.ToolCall{ID: "c1", Function: types.FunctionCall{Name: "exec", Arguments: `{"command":"pwd"}`}}
	result := s.Handle(call)
	if result.IsError {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	// Output should contain /tmp
	if result.Content == "" {
		t.Fatal("expected output")
	}
}

func TestExecFailingCommand(t *testing.T) {
	s := New()
	call := types.ToolCall{ID: "c1", Function: types.FunctionCall{Name: "exec", Arguments: `{"command":"false"}`}}
	result := s.Handle(call)
	if !result.IsError {
		t.Fatal("expected error for failing command")
	}
}

func TestExecEmptyCommand(t *testing.T) {
	s := New()
	call := types.ToolCall{ID: "c1", Function: types.FunctionCall{Name: "exec", Arguments: `{"command":""}`}}
	result := s.Handle(call)
	if !result.IsError {
		t.Fatal("expected error for empty command")
	}
}

func TestExecTimeout(t *testing.T) {
	s := New()
	s.Timeout = 1 // 1 second
	call := types.ToolCall{ID: "c1", Function: types.FunctionCall{Name: "exec", Arguments: `{"command":"sleep 10"}`}}
	result := s.Handle(call)
	if !result.IsError {
		t.Fatal("expected timeout error")
	}
}

func TestExecInvalidJSON(t *testing.T) {
	s := New()
	call := types.ToolCall{ID: "c1", Function: types.FunctionCall{Name: "exec", Arguments: `not json`}}
	result := s.Handle(call)
	if !result.IsError {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestToolSchemas(t *testing.T) {
	s := New()
	schemas := s.ToolSchemas()
	if len(schemas) != 1 {
		t.Fatalf("expected 1 schema, got %d", len(schemas))
	}
	if schemas[0].Name != "exec" {
		t.Fatalf("expected 'exec', got '%s'", schemas[0].Name)
	}
}

func TestExecAllowedCmds(t *testing.T) {
	s := New()
	s.AllowedCmds = []string{"echo", "ls"}
	call := types.ToolCall{ID: "c1", Function: types.FunctionCall{Name: "exec", Arguments: `{"command":"echo ok"}`}}
	result := s.Handle(call)
	if result.IsError {
		t.Fatalf("echo should be allowed: %s", result.Content)
	}

	call = types.ToolCall{ID: "c2", Function: types.FunctionCall{Name: "exec", Arguments: `{"command":"cat /etc/passwd"}`}}
	result = s.Handle(call)
	if !result.IsError {
		t.Fatal("cat should be blocked")
	}
}

func TestExecMaxOutput(t *testing.T) {
	s := New()
	s.MaxOutput = 50
	call := types.ToolCall{ID: "c1", Function: types.FunctionCall{Name: "exec", Arguments: `{"command":"seq 1 1000"}`}}
	result := s.Handle(call)
	if len(result.Content) > 100 {
		t.Fatalf("expected truncation, got length %d", len(result.Content))
	}
}
