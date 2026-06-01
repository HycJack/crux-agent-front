package context

import (
	"testing"

	"github.com/hermes-go/core/types"
)

func TestBuildWithMemory(t *testing.T) {
	mgr := NewWithBudget("Base prompt", 1000)
	mgr.SetMemory("User prefers concise responses")

	msgs := mgr.Build([]types.Message{{Role: "user", Content: "hello"}})
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Fatalf("first message should be system, got %s", msgs[0].Role)
	}
	if msgs[0].Content != "Base prompt\n\n[Memory Context]\nUser prefers concise responses" {
		t.Fatalf("unexpected system prompt: %s", msgs[0].Content)
	}
}

func TestBuildWithSkills(t *testing.T) {
	mgr := New("Base")
	mgr.SetSkillDescriptions([]string{"terminal: execute shell commands", "memory: persistent storage"})

	msgs := mgr.Build([]types.Message{{Role: "user", Content: "hi"}})
	if len(msgs) != 2 {
		t.Fatalf("expected 2, got %d", len(msgs))
	}
	if !containsSubstring(msgs[0].Content, "[Available Skills]") {
		t.Fatal("expected skill descriptions in system prompt")
	}
}

func TestBuildWithMemoryAndSkills(t *testing.T) {
	mgr := New("Base")
	mgr.SetMemory("test memory")
	mgr.SetSkillDescriptions([]string{"skill1"})

	msgs := mgr.Build([]types.Message{{Role: "user", Content: "hi"}})
	if len(msgs) != 2 {
		t.Fatalf("expected 2, got %d", len(msgs))
	}
	content := msgs[0].Content
	if !containsSubstring(content, "[Memory Context]") {
		t.Fatal("expected memory in system prompt")
	}
	if !containsSubstring(content, "[Available Skills]") {
		t.Fatal("expected skills in system prompt")
	}
}

func TestStripSystem(t *testing.T) {
	mgr := New("system")
	msgs := []types.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hello"},
	}
	stripped := mgr.Strip(msgs)
	if len(stripped) != 1 {
		t.Fatalf("expected 1, got %d", len(stripped))
	}
	if stripped[0].Role != "user" {
		t.Fatalf("expected user, got %s", stripped[0].Role)
	}
}

func TestTrim(t *testing.T) {
	mgr := NewWithBudget("system", 50) // very small budget
	msgs := []types.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "a very long message that should be trimmed because it exceeds the token budget significantly"},
		{Role: "assistant", Content: "another very long message that also exceeds the budget and should be trimmed from context"},
		{Role: "user", Content: "yet another long message to ensure we have enough to trim from the conversation history"},
	}
	trimmed := mgr.Trim(msgs)
	if len(trimmed) >= len(msgs) {
		t.Fatalf("expected trimming to reduce messages, got %d vs %d", len(trimmed), len(msgs))
	}
}

func TestTokenBudget(t *testing.T) {
	mgr := NewWithBudget("system", 50000)
	if mgr.TokenBudget() != 50000 {
		t.Fatalf("expected 50000, got %d", mgr.TokenBudget())
	}
}

func TestEstimateTokens(t *testing.T) {
	msgs := []types.Message{
		{Role: "user", Content: "hello world"},
	}
	tokens := EstimateTokens(msgs)
	if tokens <= 0 {
		t.Fatalf("expected positive tokens, got %d", tokens)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
