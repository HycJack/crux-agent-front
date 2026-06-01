package agent

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestRunBasic(t *testing.T) {
	a := New(AgentLoopConfig{
		StreamFn: func(ctx context.Context, msgs []AgentMessage, tools []AgentTool) (*StreamResult, error) {
			return &StreamResult{Message: AgentMessage{Role: "assistant", Content: "hello"}}, nil
		},
	})
	result, err := a.Run([]AgentMessage{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	if result[1].Content != "hello" {
		t.Fatalf("expected hello, got %s", result[1].Content)
	}
}

func TestRunToolCall(t *testing.T) {
	callCount := 0
	a := New(AgentLoopConfig{
		Tools: []AgentTool{{
			Name: "echo",
			Execute: func(c AgentToolCall) AgentToolResult {
				return AgentToolResult{ToolCallID: c.ID, Content: "echoed"}
			},
		}},
		StreamFn: func(ctx context.Context, msgs []AgentMessage, tools []AgentTool) (*StreamResult, error) {
			callCount++
			if callCount == 1 {
				return &StreamResult{Message: AgentMessage{Role: "assistant", ToolCalls: []AgentToolCall{
					{ID: "c1", Type: "function", Function: FunctionCall{Name: "echo", Arguments: "{}"}},
				}}}, nil
			}
			return &StreamResult{Message: AgentMessage{Role: "assistant", Content: "done"}}, nil
		},
	})
	result, err := a.Run([]AgentMessage{{Role: "user", Content: "echo"}})
	if err != nil {
		t.Fatal(err)
	}
	// user + assistant(tool_call) + tool + assistant(done) = 4
	if len(result) != 4 {
		t.Fatalf("expected 4, got %d", len(result))
	}
}

func TestRunParallelTools(t *testing.T) {
	callCount := 0
	a := New(AgentLoopConfig{
		ToolMode: ToolParallel,
		Tools: []AgentTool{
			{Name: "a", Execute: func(c AgentToolCall) AgentToolResult { return AgentToolResult{ToolCallID: c.ID, Content: "a"} }},
			{Name: "b", Execute: func(c AgentToolCall) AgentToolResult { return AgentToolResult{ToolCallID: c.ID, Content: "b"} }},
		},
		StreamFn: func(ctx context.Context, msgs []AgentMessage, tools []AgentTool) (*StreamResult, error) {
			callCount++
			if callCount == 1 {
				return &StreamResult{Message: AgentMessage{Role: "assistant", ToolCalls: []AgentToolCall{
					{ID: "c1", Type: "function", Function: FunctionCall{Name: "a", Arguments: "{}"}},
					{ID: "c2", Type: "function", Function: FunctionCall{Name: "b", Arguments: "{}"}},
				}}}, nil
			}
			return &StreamResult{Message: AgentMessage{Role: "assistant", Content: "done"}}, nil
		},
	})
	result, err := a.Run([]AgentMessage{{Role: "user", Content: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 5 {
		t.Fatalf("expected 5, got %d", len(result))
	}
}

func TestRunMaxRounds(t *testing.T) {
	a := New(AgentLoopConfig{
		MaxRounds: 3,
		StreamFn: func(ctx context.Context, msgs []AgentMessage, tools []AgentTool) (*StreamResult, error) {
			return &StreamResult{Message: AgentMessage{Role: "assistant", ToolCalls: []AgentToolCall{
				{ID: "c1", Type: "function", Function: FunctionCall{Name: "noop", Arguments: "{}"}},
			}}}, nil
		},
	})
	_, err := a.Run([]AgentMessage{{Role: "user", Content: "test"}})
	if err == nil {
		t.Fatal("expected max rounds error")
	}
}

func TestRunAbort(t *testing.T) {
	a := New(AgentLoopConfig{
		MaxRounds: 100,
		StreamFn: func(ctx context.Context, msgs []AgentMessage, tools []AgentTool) (*StreamResult, error) {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return &StreamResult{Message: AgentMessage{Role: "assistant", ToolCalls: []AgentToolCall{
				{ID: "c1", Type: "function", Function: FunctionCall{Name: "noop", Arguments: "{}"}},
			}}}, nil
		},
	})
	go func() { a.Abort() }()
	_, _ = a.Run([]AgentMessage{{Role: "user", Content: "test"}})
}

func TestRunBeforeToolHook(t *testing.T) {
	blocked := false
	callCount := 0
	a := New(AgentLoopConfig{
		Tools: []AgentTool{{
			Name: "test",
			Execute: func(c AgentToolCall) AgentToolResult {
				return AgentToolResult{ToolCallID: c.ID, Content: "ok"}
			},
		}},
		StreamFn: func(ctx context.Context, msgs []AgentMessage, tools []AgentTool) (*StreamResult, error) {
			callCount++
			if callCount == 1 {
				return &StreamResult{Message: AgentMessage{Role: "assistant", ToolCalls: []AgentToolCall{
					{ID: "c1", Type: "function", Function: FunctionCall{Name: "test", Arguments: "{}"}},
				}}}, nil
			}
			return &StreamResult{Message: AgentMessage{Role: "assistant", Content: "done"}}, nil
		},
		BeforeToolCall: func(ctx BeforeToolCallContext) *BeforeToolCallResult {
			blocked = true
			return &BeforeToolCallResult{Block: true, Reason: "blocked"}
		},
	})
	result, err := a.Run([]AgentMessage{{Role: "user", Content: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	if !blocked {
		t.Fatal("hook not called")
	}
	// Check blocked message
	found := false
	for _, msg := range result {
		if msg.Role == "tool" && msg.Content == "Blocked: blocked" {
			found = true
		}
	}
	if !found {
		t.Fatal("blocked message not found")
	}
}

func TestRunAfterToolHook(t *testing.T) {
	overridden := false
	callCount := 0
	a := New(AgentLoopConfig{
		Tools: []AgentTool{{
			Name: "test",
			Execute: func(c AgentToolCall) AgentToolResult {
				return AgentToolResult{ToolCallID: c.ID, Content: "original"}
			},
		}},
		StreamFn: func(ctx context.Context, msgs []AgentMessage, tools []AgentTool) (*StreamResult, error) {
			callCount++
			if callCount == 1 {
				return &StreamResult{Message: AgentMessage{Role: "assistant", ToolCalls: []AgentToolCall{
					{ID: "c1", Type: "function", Function: FunctionCall{Name: "test", Arguments: "{}"}},
				}}}, nil
			}
			return &StreamResult{Message: AgentMessage{Role: "assistant", Content: "done"}}, nil
		},
		AfterToolCall: func(ctx AfterToolCallContext) *AfterToolCallResult {
			overridden = true
			newContent := "overridden"
			return &AfterToolCallResult{Content: &newContent}
		},
	})
	result, err := a.Run([]AgentMessage{{Role: "user", Content: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	if !overridden {
		t.Fatal("hook not called")
	}
	for _, msg := range result {
		if msg.Role == "tool" && msg.Content != "overridden" {
			t.Fatalf("expected overridden, got %s", msg.Content)
		}
	}
}

func TestRunTerminate(t *testing.T) {
	callCount := 0
	a := New(AgentLoopConfig{
		Tools: []AgentTool{{
			Name: "test",
			Execute: func(c AgentToolCall) AgentToolResult {
				return AgentToolResult{ToolCallID: c.ID, Content: "done", Terminate: true}
			},
		}},
		StreamFn: func(ctx context.Context, msgs []AgentMessage, tools []AgentTool) (*StreamResult, error) {
			callCount++
			if callCount == 1 {
				return &StreamResult{Message: AgentMessage{Role: "assistant", ToolCalls: []AgentToolCall{
					{ID: "c1", Type: "function", Function: FunctionCall{Name: "test", Arguments: "{}"}},
				}}}, nil
			}
			return &StreamResult{Message: AgentMessage{Role: "assistant", Content: "should not reach"}}, nil
		},
	})
	result, err := a.Run([]AgentMessage{{Role: "user", Content: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	// Should stop after tool returns terminate, not call LLM again
	if callCount != 1 {
		t.Fatalf("expected 1 LLM call, got %d", callCount)
	}
	_ = result
}

func TestRunUnknownTool(t *testing.T) {
	callCount := 0
	a := New(AgentLoopConfig{
		StreamFn: func(ctx context.Context, msgs []AgentMessage, tools []AgentTool) (*StreamResult, error) {
			callCount++
			if callCount == 1 {
				return &StreamResult{Message: AgentMessage{Role: "assistant", ToolCalls: []AgentToolCall{
					{ID: "c1", Type: "function", Function: FunctionCall{Name: "nonexistent", Arguments: "{}"}},
				}}}, nil
			}
			return &StreamResult{Message: AgentMessage{Role: "assistant", Content: "done"}}, nil
		},
	})
	result, err := a.Run([]AgentMessage{{Role: "user", Content: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, msg := range result {
		if msg.Role == "tool" && msg.Content == "Unknown tool: nonexistent" {
			found = true
		}
	}
	if !found {
		t.Fatal("unknown tool error not found")
	}
}

func TestSteer(t *testing.T) {
	a := New(AgentLoopConfig{
		StreamFn: func(ctx context.Context, msgs []AgentMessage, tools []AgentTool) (*StreamResult, error) {
			return &StreamResult{Message: AgentMessage{Role: "assistant", Content: "done"}}, nil
		},
	})
	a.Steer(AgentMessage{Role: "user", Content: "steered"})
	if len(a.steeringQueue) != 1 {
		t.Fatalf("expected 1, got %d", len(a.steeringQueue))
	}
}

func TestFollowUp(t *testing.T) {
	a := New(AgentLoopConfig{
		StreamFn: func(ctx context.Context, msgs []AgentMessage, tools []AgentTool) (*StreamResult, error) {
			return &StreamResult{Message: AgentMessage{Role: "assistant", Content: "done"}}, nil
		},
	})
	a.FollowUp(AgentMessage{Role: "user", Content: "followed"})
	if len(a.followUpQueue) != 1 {
		t.Fatalf("expected 1, got %d", len(a.followUpQueue))
	}
}

func TestIsRunning(t *testing.T) {
	a := New(AgentLoopConfig{
		StreamFn: func(ctx context.Context, msgs []AgentMessage, tools []AgentTool) (*StreamResult, error) {
			return &StreamResult{Message: AgentMessage{Role: "assistant", Content: "done"}}, nil
		},
	})
	if a.IsRunning() {
		t.Fatal("should not be running before Run")
	}
}

func TestOnEvent(t *testing.T) {
	var events atomic.Int32
	a := New(AgentLoopConfig{
		StreamFn: func(ctx context.Context, msgs []AgentMessage, tools []AgentTool) (*StreamResult, error) {
			return &StreamResult{Message: AgentMessage{Role: "assistant", Content: "done"}}, nil
		},
		OnEvent: func(event AgentEvent) {
			events.Add(1)
		},
	})
	a.Run([]AgentMessage{{Role: "user", Content: "test"}})
	if events.Load() < 2 {
		t.Fatalf("expected at least 2 events, got %d", events.Load())
	}
}

func TestConvertToLlm(t *testing.T) {
	msgs := []AgentMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
		{Role: "compactionSummary", Summary: "old"},
		{Role: "branchSummary", Summary: "branch"},
		{Role: "custom", CustomType: "note", Content: "custom", Display: true},
		{Role: "custom", CustomType: "hidden", Content: "hidden", Display: false},
		{Role: "assistant", Content: "ok"},
	}
	converted := ConvertToLlm(msgs)
	// system, user, compaction->user, branch->user, custom(display)->user, assistant = 6
	if len(converted) != 6 {
		t.Fatalf("expected 6, got %d", len(converted))
	}
}

func TestInjectMemory(t *testing.T) {
	msgs := []AgentMessage{{Role: "system", Content: "sys"}, {Role: "user", Content: "hi"}}
	result := ApplyTransforms(msgs, InjectMemory("test memory"))
	if len(result) != 3 {
		t.Fatalf("expected 3, got %d", len(result))
	}
	if result[1].Content != "[Memory Context]\ntest memory" {
		t.Fatalf("unexpected: %s", result[1].Content)
	}
}

func TestInjectMemoryEmpty(t *testing.T) {
	msgs := []AgentMessage{{Role: "user", Content: "hi"}}
	result := ApplyTransforms(msgs, InjectMemory(""))
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
}

func TestInjectSkills(t *testing.T) {
	msgs := []AgentMessage{{Role: "system", Content: "sys"}}
	result := ApplyTransforms(msgs, InjectSkills("skill desc"))
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
}

func TestTrimToBudget(t *testing.T) {
	msgs := []AgentMessage{
		{Role: "user", Content: "a very long message that should be trimmed because it exceeds the budget significantly"},
		{Role: "assistant", Content: "another very long message that also exceeds the budget and should be trimmed"},
		{Role: "user", Content: "yet another long message"},
	}
	estimator := func(msgs []AgentMessage) int {
		total := 0
		for _, m := range msgs {
			total += len(m.Content) / 2
		}
		return total
	}
	result := ApplyTransforms(msgs, TrimToBudget(20, estimator))
	if len(result) >= len(msgs) {
		t.Fatal("expected trimming")
	}
}

func TestFormatCompactionSummary(t *testing.T) {
	msg := AgentMessage{Role: "compactionSummary", Summary: "test summary", TokensBefore: 1000}
	s := formatCompactionSummary(msg)
	if s == "" {
		t.Fatal("expected non-empty")
	}
}

func TestFormatBranchSummary(t *testing.T) {
	msg := AgentMessage{Role: "branchSummary", Summary: "branch summary"}
	s := formatBranchSummary(msg)
	if s == "" {
		t.Fatal("expected non-empty")
	}
}

func TestCreateMessages(t *testing.T) {
	branch := CreateBranchSummaryMessage("summary", "from-1", 1000)
	if branch.Role != "branchSummary" {
		t.Fatal("wrong role")
	}
	compact := CreateCompactionSummaryMessage("summary", 500, 1000)
	if compact.Role != "compactionSummary" {
		t.Fatal("wrong role")
	}
	custom := CreateCustomMessage("note", "content", true, 1000)
	if custom.Role != "custom" {
		t.Fatal("wrong role")
	}
}
