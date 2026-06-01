package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hermes-go/core/agent"
	"github.com/hermes-go/core/result"
)

func TestSessionBuildContext(t *testing.T) {
	storage := NewMemoryStorage()
	session := NewSession(storage)

	session.AppendMessage(agent.AgentMessage{Role: "user", Content: "hello"})
	session.AppendMessage(agent.AgentMessage{Role: "assistant", Content: "hi!"})

	messages, err := session.BuildContext()
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
}

func TestSessionCompaction(t *testing.T) {
	storage := NewMemoryStorage()
	session := NewSession(storage)

	session.AppendMessage(agent.AgentMessage{Role: "user", Content: "old message 1"})
	session.AppendMessage(agent.AgentMessage{Role: "assistant", Content: "old reply 1"})

	compEntry := SessionTreeEntry{
		ID:           "comp-1",
		Type:         "compaction",
		Summary:      "Previous conversation about old topics",
		TokensBefore: 100,
	}
	storage.AppendEntries([]SessionTreeEntry{compEntry})

	session.AppendMessage(agent.AgentMessage{Role: "user", Content: "new message"})

	messages, err := session.BuildContext()
	if err != nil {
		t.Fatal(err)
	}

	if len(messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(messages))
	}
	if messages[0].Role != "compactionSummary" {
		t.Fatalf("first message should be compactionSummary, got %s", messages[0].Role)
	}
}

func TestSessionFork(t *testing.T) {
	storage := NewMemoryStorage()
	session := NewSession(storage)
	session.AppendMessage(agent.AgentMessage{Role: "user", Content: "original"})

	forked, err := session.Fork("")
	if err != nil {
		t.Fatal(err)
	}
	if forked == nil {
		t.Fatal("expected non-nil forked session")
	}
}

func TestLoadSkills(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: test-skill
description: A test skill
---
# Test Skill
This is a test.`), 0644)

	skills, warnings := LoadSkills(dir)
	if len(warnings) > 0 {
		t.Logf("warnings: %v", warnings)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "test-skill" {
		t.Fatalf("expected 'test-skill', got '%s'", skills[0].Name)
	}
}

func TestFormatSkillsForSystemPrompt(t *testing.T) {
	skills := []Skill{
		{Name: "terminal", Description: "Execute shell commands", FilePath: "/skills/terminal/SKILL.md"},
	}
	prompt := FormatSkillsForSystemPrompt(skills)
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !containsStr(prompt, "<available_skills>") {
		t.Fatal("expected <available_skills> tag")
	}
	if !containsStr(prompt, "terminal") {
		t.Fatal("expected skill name")
	}
}

func TestCompactorShouldCompact(t *testing.T) {
	compactor := NewCompactor(DefaultCompactionSettings, nil)

	small := []agent.AgentMessage{{Role: "user", Content: "hello"}}
	if compactor.ShouldCompact(small) {
		t.Fatal("should not compact small messages")
	}
}

func TestCompactorGenerateTruncateSummary(t *testing.T) {
	messages := []agent.AgentMessage{
		{Role: "user", Content: "message 1"},
		{Role: "assistant", Content: "reply 1"},
		{Role: "user", Content: "message 2"},
	}
	summary := generateTruncateSummary(messages)
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
	if !containsStr(summary, "message 1") {
		t.Fatal("expected summary to contain message content")
	}
}

func TestEstimateTokens(t *testing.T) {
	msgs := []agent.AgentMessage{
		{Role: "user", Content: "hello world"},
	}
	tokens := estimateTokens(msgs)
	if tokens <= 0 {
		t.Fatalf("expected positive tokens, got %d", tokens)
	}
}

func TestAgentCoreRun(t *testing.T) {
	tools := []agent.AgentTool{
		{
			Name:        "echo",
			Description: "Echo input",
			Execute: func(call agent.AgentToolCall) agent.AgentToolResult {
				return agent.AgentToolResult{ToolCallID: call.ID, Content: "echoed"}
			},
		},
	}

	callCount := 0
	a := agent.New(agent.AgentLoopConfig{
		Tools: tools,
		StreamFn: func(ctx context.Context, messages []agent.AgentMessage, tools []agent.AgentTool) (*agent.StreamResult, error) {
			callCount++
			if callCount == 1 {
				return &agent.StreamResult{
					Message: agent.AgentMessage{
						Role:      "assistant",
						ToolCalls: []agent.AgentToolCall{{ID: "c1", Type: "function", Function: agent.FunctionCall{Name: "echo", Arguments: "{}"}}},
					},
				}, nil
			}
			return &agent.StreamResult{
				Message: agent.AgentMessage{Role: "assistant", Content: "done"},
			}, nil
		},
	})

	result, err := a.Run([]agent.AgentMessage{{Role: "user", Content: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(result))
	}
}

func TestAgentCoreToolHooks(t *testing.T) {
	tools := []agent.AgentTool{
		{
			Name: "test_tool",
			Execute: func(call agent.AgentToolCall) agent.AgentToolResult {
				return agent.AgentToolResult{ToolCallID: call.ID, Content: "ok"}
			},
		},
	}

	blocked := false
	callCount := 0
	a := agent.New(agent.AgentLoopConfig{
		Tools: tools,
		StreamFn: func(ctx context.Context, messages []agent.AgentMessage, tools []agent.AgentTool) (*agent.StreamResult, error) {
			callCount++
			if callCount == 1 {
				return &agent.StreamResult{
					Message: agent.AgentMessage{
						Role:      "assistant",
						ToolCalls: []agent.AgentToolCall{{ID: "c1", Type: "function", Function: agent.FunctionCall{Name: "test_tool", Arguments: "{}"}}},
					},
				}, nil
			}
			return &agent.StreamResult{
				Message: agent.AgentMessage{Role: "assistant", Content: "tool was blocked"},
			}, nil
		},
		BeforeToolCall: func(ctx agent.BeforeToolCallContext) *agent.BeforeToolCallResult {
			blocked = true
			return &agent.BeforeToolCallResult{Block: true, Reason: "test block"}
		},
	})

	result, err := a.Run([]agent.AgentMessage{{Role: "user", Content: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	if !blocked {
		t.Fatal("beforeToolCall should have been called")
	}
	if len(result) < 4 {
		t.Fatalf("expected at least 4 messages, got %d", len(result))
	}
}

func TestAgentParallelExecution(t *testing.T) {
	tools := []agent.AgentTool{
		{
			Name: "tool_a",
			Execute: func(call agent.AgentToolCall) agent.AgentToolResult {
				return agent.AgentToolResult{ToolCallID: call.ID, Content: "a"}
			},
		},
		{
			Name: "tool_b",
			Execute: func(call agent.AgentToolCall) agent.AgentToolResult {
				return agent.AgentToolResult{ToolCallID: call.ID, Content: "b"}
			},
		},
	}

	callCount := 0
	a := agent.New(agent.AgentLoopConfig{
		Tools:    tools,
		ToolMode: agent.ToolParallel,
		StreamFn: func(ctx context.Context, messages []agent.AgentMessage, tools []agent.AgentTool) (*agent.StreamResult, error) {
			callCount++
			if callCount == 1 {
				return &agent.StreamResult{
					Message: agent.AgentMessage{
						Role: "assistant",
						ToolCalls: []agent.AgentToolCall{
							{ID: "c1", Type: "function", Function: agent.FunctionCall{Name: "tool_a", Arguments: "{}"}},
							{ID: "c2", Type: "function", Function: agent.FunctionCall{Name: "tool_b", Arguments: "{}"}},
						},
					},
				}, nil
			}
			return &agent.StreamResult{
				Message: agent.AgentMessage{Role: "assistant", Content: "done"},
			}, nil
		},
	})

	result, err := a.Run([]agent.AgentMessage{{Role: "user", Content: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	// user + assistant(2 tool_calls) + tool_a + tool_b + assistant(done)
	if len(result) < 5 {
		t.Fatalf("expected at least 5 messages, got %d", len(result))
	}
}

func TestAgentSteering(t *testing.T) {
	a := agent.New(agent.AgentLoopConfig{
		StreamFn: func(ctx context.Context, messages []agent.AgentMessage, tools []agent.AgentTool) (*agent.StreamResult, error) {
			return &agent.StreamResult{
				Message: agent.AgentMessage{Role: "assistant", Content: "done"},
			}, nil
		},
	})

	a.Steer(agent.AgentMessage{Role: "user", Content: "steer message"})
	a.FollowUp(agent.AgentMessage{Role: "user", Content: "follow up"})

	_, err := a.Run([]agent.AgentMessage{{Role: "user", Content: "test"}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAgentAbort(t *testing.T) {
	a := agent.New(agent.AgentLoopConfig{
		MaxRounds: 100,
		StreamFn: func(ctx context.Context, messages []agent.AgentMessage, tools []agent.AgentTool) (*agent.StreamResult, error) {
			// Check context
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return &agent.StreamResult{
				Message: agent.AgentMessage{
					Role:      "assistant",
					ToolCalls: []agent.AgentToolCall{{ID: "c1", Type: "function", Function: agent.FunctionCall{Name: "noop", Arguments: "{}"}}},
				},
			}, nil
		},
	})

	go func() {
		a.Abort()
	}()

	_, _ = a.Run([]agent.AgentMessage{{Role: "user", Content: "test"}})
}

func TestConvertToLlm(t *testing.T) {
	messages := []agent.AgentMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hello"},
		{Role: "compactionSummary", Summary: "old stuff", TokensBefore: 1000},
		{Role: "assistant", Content: "hi"},
		{Role: "custom", CustomType: "note", Content: "custom content", Display: true},
		{Role: "branchSummary", Summary: "branch stuff"},
	}

	converted := agent.ConvertToLlm(messages)
	// system, user, compactionSummary->user, assistant, custom->user, branchSummary->user
	if len(converted) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(converted))
	}
	// compactionSummary should become user
	if converted[2].Role != "user" {
		t.Fatalf("expected compactionSummary to become user, got %s", converted[2].Role)
	}
}

func TestContextTransforms(t *testing.T) {
	messages := []agent.AgentMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hello"},
	}

	// Test InjectMemory
	transformed := agent.ApplyTransforms(messages, agent.InjectMemory("test memory"))
	if len(transformed) != 3 {
		t.Fatalf("expected 3 messages after InjectMemory, got %d", len(transformed))
	}

	// Test InjectSkills
	transformed = agent.ApplyTransforms(messages, agent.InjectSkills("skill desc"))
	if len(transformed) != 3 {
		t.Fatalf("expected 3 messages after InjectSkills, got %d", len(transformed))
	}
}

func TestPromptTemplates(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "greeting.md"), []byte(`---
name: greeting
description: A greeting template
---
Hello {{name}}, welcome to {{place}}!`), 0644)

	templates, err := agent.LoadTemplates(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 1 {
		t.Fatalf("expected 1 template, got %d", len(templates))
	}

	result := agent.FormatInvocation(templates[0], map[string]string{
		"name":  "Alice",
		"place": "Wonderland",
	})
	if result != "Hello Alice, welcome to Wonderland!" {
		t.Fatalf("unexpected result: %s", result)
	}
}

func TestBranchSummary(t *testing.T) {
	bs := NewBranchSummarizer(nil)
	messages := []agent.AgentMessage{
		{Role: "user", Content: "branch message 1"},
		{Role: "assistant", Content: "branch reply 1"},
	}
	summary, err := bs.SummarizeBranch(messages)
	if err != nil {
		t.Fatal(err)
	}
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
}

func TestResultPattern(t *testing.T) {
	// Test Ok
	okResult := result.Ok[int, error](42)
	if !okResult.Ok {
		t.Fatal("expected ok")
	}
	if v, ok := okResult.Unwrap(); !ok || v != 42 {
		t.Fatalf("expected 42, got %d", v)
	}

	// Test Error
	errResult := result.Error[int, error](fmt.Errorf("test error"))
	if errResult.Ok {
		t.Fatal("expected error")
	}
	if v, ok := errResult.Unwrap(); ok {
		t.Fatalf("expected no value, got %d", v)
	}

	// Test Map
	mapped := result.Map(okResult, func(v int) string { return fmt.Sprintf("value=%d", v) })
	if mapped.GetOrDefault("") != "value=42" {
		t.Fatalf("unexpected mapped value: %s", mapped.GetOrDefault(""))
	}

	// Test FlatMap
	flatMapped := result.FlatMap(okResult, func(v int) result.Result[string, error] {
		return result.Ok[string, error](fmt.Sprintf("flat=%d", v))
	})
	if flatMapped.GetOrDefault("") != "flat=42" {
		t.Fatalf("unexpected flatMapped value: %s", flatMapped.GetOrDefault(""))
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
