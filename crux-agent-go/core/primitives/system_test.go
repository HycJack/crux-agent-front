package primitives

import (
	"encoding/json"
	"os"
	"runtime"
	"testing"

	"github.com/hermes-go/core/types"
)

func TestGetTime(t *testing.T) {
	sp := NewSystem()
	result := sp.Handle(types.ToolCall{
		ID: "c1", Function: types.FunctionCall{Name: "get_time", Arguments: "{}"},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !containsStr(result.Content, "UTC:") {
		t.Errorf("missing UTC: %s", result.Content)
	}
	if !containsStr(result.Content, "Unix:") {
		t.Errorf("missing Unix: %s", result.Content)
	}
}

func TestGetOS(t *testing.T) {
	sp := NewSystem()
	result := sp.Handle(types.ToolCall{
		ID: "c1", Function: types.FunctionCall{Name: "get_os", Arguments: "{}"},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !containsStr(result.Content, runtime.GOOS) {
		t.Errorf("missing OS %s in: %s", runtime.GOOS, result.Content)
	}
	if !containsStr(result.Content, runtime.GOARCH) {
		t.Errorf("missing arch %s in: %s", runtime.GOARCH, result.Content)
	}
}

func TestGetCWD(t *testing.T) {
	sp := NewSystem()
	result := sp.Handle(types.ToolCall{
		ID: "c1", Function: types.FunctionCall{Name: "get_cwd", Arguments: "{}"},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	cwd, _ := os.Getwd()
	if result.Content != cwd {
		t.Errorf("expected %s, got %s", cwd, result.Content)
	}
}

func TestGetEnv(t *testing.T) {
	os.Setenv("HERMES_TEST_VAR", "test-value-123")
	defer os.Unsetenv("HERMES_TEST_VAR")

	sp := NewSystem()
	args, _ := json.Marshal(map[string]string{"key": "HERMES_TEST_VAR"})
	result := sp.Handle(types.ToolCall{
		ID: "c1", Function: types.FunctionCall{Name: "get_env", Arguments: string(args)},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "test-value-123" {
		t.Errorf("expected test-value-123, got %s", result.Content)
	}
}

func TestGetEnvNotSet(t *testing.T) {
	sp := NewSystem()
	args, _ := json.Marshal(map[string]string{"key": "HERMES_NONEXISTENT_VAR"})
	result := sp.Handle(types.ToolCall{
		ID: "c1", Function: types.FunctionCall{Name: "get_env", Arguments: string(args)},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !containsStr(result.Content, "not set") {
		t.Errorf("expected 'not set' message, got %s", result.Content)
	}
}

func TestSystemToolSchemas(t *testing.T) {
	sp := NewSystem()
	schemas := sp.ToolSchemas()
	if len(schemas) != 4 {
		t.Fatalf("expected 4 schemas, got %d", len(schemas))
	}
}
