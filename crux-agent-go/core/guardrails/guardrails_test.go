package guardrails

import (
	"testing"

	"github.com/hermes-go/core/types"
)

func TestCheckInput(t *testing.T) {
	g := New(Config{MaxInputLength: 100})
	if err := g.CheckInput("hello"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCheckInputTooLong(t *testing.T) {
	g := New(Config{MaxInputLength: 5})
	if err := g.CheckInput("hello world"); err == nil {
		t.Fatal("expected error for long input")
	}
}

func TestCheckInputBlockedPattern(t *testing.T) {
	g := New(Config{BlockedPatterns: []string{`rm -rf`}})
	if err := g.CheckInput("please rm -rf /"); err == nil {
		t.Fatal("expected error for blocked pattern")
	}
}

func TestCheckInputPII(t *testing.T) {
	g := New(Config{PIIDetection: true})
	if err := g.CheckInput("my email is test@example.com"); err == nil {
		t.Fatal("expected PII detection error")
	}
}

func TestCheckOutput(t *testing.T) {
	g := New(Config{MaxOutputLength: 50})
	result := g.CheckOutput("tool", types.ToolResult{
		Content: "this is a very long output that exceeds the limit and should be truncated by guardrails",
	})
	if len(result.Content) > 100 {
		t.Fatalf("expected truncation, got length %d", len(result.Content))
	}
	if !containsSubstring(result.Content, "truncated") {
		t.Fatal("expected truncation marker")
	}
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestCheckToolCall(t *testing.T) {
	g := New(Config{BlockedCommands: []string{"rm -rf"}})
	call := types.ToolCall{
		ID:       "c1",
		Function: types.FunctionCall{Name: "exec", Arguments: `{"command":"rm -rf /"}`},
	}
	if err := g.CheckToolCall(call); err == nil {
		t.Fatal("expected error for blocked command")
	}
}

func TestCheckToolCallAllowed(t *testing.T) {
	g := New(Config{})
	call := types.ToolCall{
		ID:       "c1",
		Function: types.FunctionCall{Name: "exec", Arguments: `{"command":"ls -la"}`},
	}
	if err := g.CheckToolCall(call); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRateLimit(t *testing.T) {
	g := New(Config{RateLimit: 2})
	call := types.ToolCall{
		ID:       "c1",
		Function: types.FunctionCall{Name: "exec", Arguments: `{"command":"echo hi"}`},
	}
	if err := g.CheckToolCall(call); err != nil {
		t.Fatalf("first call should pass: %v", err)
	}
	if err := g.CheckToolCall(call); err != nil {
		t.Fatalf("second call should pass: %v", err)
	}
	if err := g.CheckToolCall(call); err == nil {
		t.Fatal("third call should be rate limited")
	}
}
