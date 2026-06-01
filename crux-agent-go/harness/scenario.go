package harness

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hermes-go/core/agentloop"
	"github.com/hermes-go/core/context"
	"github.com/hermes-go/core/llm"
	"github.com/hermes-go/core/types"
)

// Scenario defines a test scenario for the agent.
type Scenario struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Messages    []types.Message   `yaml:"-"` // Input messages
	Responses   []llm.Response    `yaml:"-"` // Mock LLM responses
	Tools       []ToolHandler     `yaml:"-"` // Available tools
	Expect      Expectation       `yaml:"-"`
}

// ToolHandler pairs a tool schema with its handler.
type ToolHandler struct {
	Schema  types.ToolSchema
	Handler func(types.ToolCall) types.ToolResult
}

// Expectation defines what to verify after a scenario run.
type Expectation struct {
	MinRounds         int
	MaxRounds         int
	ResponseContains  string
	ToolCallsExpected []string // Tool names that should be called
	NoError           bool
}

// ScenarioResult holds the outcome of running a scenario.
type ScenarioResult struct {
	Scenario  string
	Passed    bool
	Messages  []types.Message
	ToolCalls []string
	Error     error
	Rounds    int
}

// RunScenario runs a single scenario and returns the result.
func RunScenario(s *Scenario) *ScenarioResult {
	result := &ScenarioResult{Scenario: s.Name}

	// Build tool list and executor
	var tools []types.ToolSchema
	handlers := make(map[string]func(types.ToolCall) types.ToolResult)
	for _, th := range s.Tools {
		tools = append(tools, th.Schema)
		handlers[th.Schema.Name] = th.Handler
	}

	executor := func(call types.ToolCall) types.ToolResult {
		fn, ok := handlers[call.Function.Name]
		if !ok {
			return types.ToolResult{
				ToolCallID: call.ID,
				Content:    fmt.Sprintf("Unknown tool: %s", call.Function.Name),
				IsError:    true,
			}
		}
		return fn(call)
	}

	// Create mock provider
	provider := NewMockProvider(s.Responses...)

	// Create loop
	loop := agentloop.New(provider, tools, executor)
	loop.MaxRounds = 20

	// Run
	messages := s.Messages
	result.Messages, result.Error = loop.Run(messages)
	result.Rounds = provider.CallCount()

	// Collect tool calls
	for _, msg := range result.Messages {
		if msg.Role == "assistant" {
			for _, tc := range msg.ToolCalls {
				result.ToolCalls = append(result.ToolCalls, tc.Function.Name)
			}
		}
	}

	// Evaluate expectations
	result.Passed = true
	if s.Expect.NoError && result.Error != nil {
		result.Passed = false
	}
	if s.Expect.MaxRounds > 0 && result.Rounds > s.Expect.MaxRounds {
		result.Passed = false
	}
	if s.Expect.MinRounds > 0 && result.Rounds < s.Expect.MinRounds {
		result.Passed = false
	}
	if s.Expect.ResponseContains != "" {
		found := false
		for _, msg := range result.Messages {
			if msg.Role == "assistant" && strings.Contains(msg.Content, s.Expect.ResponseContains) {
				found = true
				break
			}
		}
		if !found {
			result.Passed = false
		}
	}
	for _, expectedTool := range s.Expect.ToolCallsExpected {
		found := false
		for _, tc := range result.ToolCalls {
			if tc == expectedTool {
				found = true
				break
			}
		}
		if !found {
			result.Passed = false
		}
	}

	return result
}

// RunScenarioT runs a scenario as a Go test.
func RunScenarioT(t *testing.T, s *Scenario) {
	t.Helper()
	result := RunScenario(s)
	if !result.Passed {
		t.Errorf("scenario %q failed: rounds=%d, tools=%v, error=%v",
			s.Name, result.Rounds, result.ToolCalls, result.Error)
	}
}

// RunContextScenario tests the context manager with memory and skills.
func RunContextScenario(t *testing.T, name string, systemPrompt string, memory string, skills []string, userMsg string) {
	t.Helper()
	mgr := context.New(systemPrompt)
	mgr.SetMemory(memory)
	mgr.SetSkillDescriptions(skills)

	msgs := mgr.Build([]types.Message{{Role: "user", Content: userMsg}})
	if len(msgs) < 2 {
		t.Errorf("scenario %q: expected at least 2 messages, got %d", name, len(msgs))
		return
	}
	if msgs[0].Role != "system" {
		t.Errorf("scenario %q: first message should be system", name)
	}
	if memory != "" && !strings.Contains(msgs[0].Content, "[Memory Context]") {
		t.Errorf("scenario %q: memory not injected", name)
	}
	if len(skills) > 0 && !strings.Contains(msgs[0].Content, "[Available Skills]") {
		t.Errorf("scenario %q: skills not injected", name)
	}
}

// RunStoreScenario tests store operations.
func RunStoreScenario(t *testing.T, name string, storeOps func(s *MockStore) error) {
	t.Helper()
	s := NewMockStore()
	if err := storeOps(s); err != nil {
		t.Errorf("store scenario %q failed: %v", name, err)
	}
}
