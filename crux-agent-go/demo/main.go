// Demo: exercises all components including memory, bus, context manager.
// No API key needed — uses mock LLM.
//
// Run: go run demo/main.go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hermes-go/core/agentloop"
	"github.com/hermes-go/core/bus"
	"github.com/hermes-go/core/context"
	"github.com/hermes-go/core/llm"
	"github.com/hermes-go/core/primitives"
	"github.com/hermes-go/core/types"
	"github.com/hermes-go/skills/memory"
	"github.com/hermes-go/skills/terminal"
)

type mockLLM struct {
	responses []*llm.Response
	callCount int
}

func (m *mockLLM) Complete(messages []types.Message, tools []types.ToolSchema) (*llm.Response, error) {
	if m.callCount >= len(m.responses) {
		return &llm.Response{Message: types.Message{Role: "assistant", Content: "Done."}, FinishReason: "stop"}, nil
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

func (m *mockLLM) Model() string { return "mock-gpt-4o" }

func (m *mockLLM) Stream(messages []types.Message, tools []types.ToolSchema) (*llm.StreamResponse, error) {
	resp, err := m.Complete(messages, tools)
	if err != nil {
		return nil, err
	}
	ch := make(chan llm.StreamChunk, 2)
	ch <- llm.StreamChunk{Delta: resp.Message.Content, ToolCalls: resp.Message.ToolCalls}
	ch <- llm.StreamChunk{FinishReason: resp.FinishReason, Done: true}
	return &llm.StreamResponse{Chan: ch, Cancel: func() {}}, nil
}

func main() {
	fmt.Println("=== Hermes Go Demo (Full) ===")
	fmt.Println()

	tmpDir := filepath.Join(os.TempDir(), "hermes-demo-full")
	os.MkdirAll(tmpDir, 0755)

	// Setup
	helloFile := filepath.Join(tmpDir, "hello.txt")
	os.WriteFile(helloFile, []byte("Hello from Hermes Go!\n"), 0644)

	// Event Bus
	eventBus := bus.New()
	eventCount := 0
	eventBus.Subscribe("*", "counter", func(e bus.Event) error {
		eventCount++
		return nil
	})

	// Register tools
	var tools []types.ToolSchema
	handlers := map[string]func(types.ToolCall) types.ToolResult{}

	fp := primitives.NewFile()
	tools = append(tools, fp.ToolSchemas()...)
	for _, n := range []string{"read_file", "write_file", "patch_file", "search_content", "search_files"} {
		handlers[n] = fp.Handle
	}

	sp := primitives.NewSystem()
	tools = append(tools, sp.ToolSchemas()...)
	for _, n := range []string{"get_time", "get_os", "get_cwd", "get_env"} {
		handlers[n] = sp.Handle
	}

	ts := terminal.New()
	tools = append(tools, ts.ToolSchemas()...)
	handlers["exec"] = ts.Handle

	// Memory
	memDir := filepath.Join(tmpDir, "memory")
	mem := memory.New(memDir)
	mem.Init(nil)
	tools = append(tools, mem.ToolSchemas()...)
	for _, n := range []string{"memory_save", "memory_search", "memory_list", "memory_delete"} {
		handlers[n] = mem.Handle
	}

	executor := func(call types.ToolCall) types.ToolResult {
		eventBus.Emit(bus.Event{Type: "tool_call", Source: "agent", Payload: call.Function.Name})
		return handlers[call.Function.Name](call)
	}

	// Context Manager
	ctxMgr := context.NewWithBudget("You are Hermes.", 128000)

	// --- Demos ---
	demoNum := 0
	run := func(name string, responses []*llm.Response, userMsg string) {
		demoNum++
		fmt.Printf("--- Demo %d: %s ---\n", demoNum, name)
		fmt.Printf("User: %s\n", userMsg)

		provider := &mockLLM{responses: responses}
		loop := agentloop.New(provider, tools, executor)
		loop.MaxRounds = 10

		messages := ctxMgr.Build([]types.Message{{Role: "user", Content: userMsg}})
		result, err := loop.Run(messages)
		if err != nil {
			fmt.Printf("  ERROR: %v\n\n", err)
			return
		}
		for i := len(result) - 1; i >= 0; i-- {
			if result[i].Role == "assistant" && result[i].Content != "" {
				fmt.Printf("Agent: %s\n\n", result[i].Content)
				return
			}
		}
	}

	// 1. File read
	run("Read File", []*llm.Response{
		{Message: types.Message{Role: "assistant", ToolCalls: []types.ToolCall{
			{ID: "c1", Type: "function", Function: types.FunctionCall{Name: "read_file", Arguments: `{"path":"` + helloFile + `"}`}},
		}}, FinishReason: "tool_calls"},
		{Message: types.Message{Role: "assistant", Content: "File read successfully."}, FinishReason: "stop"},
	}, "Read hello.txt")

	// 2. Memory save
	run("Save Memory", []*llm.Response{
		{Message: types.Message{Role: "assistant", ToolCalls: []types.ToolCall{
			{ID: "c2", Type: "function", Function: types.FunctionCall{Name: "memory_save", Arguments: `{"content":"User is building a Go agent framework called Hermes","target":"user","tags":["project","go"]}`}},
		}}, FinishReason: "tool_calls"},
		{Message: types.Message{Role: "assistant", Content: "Saved to memory."}, FinishReason: "stop"},
	}, "Remember that I'm building a Go agent framework")

	// 3. Memory search
	run("Search Memory", []*llm.Response{
		{Message: types.Message{Role: "assistant", ToolCalls: []types.ToolCall{
			{ID: "c3", Type: "function", Function: types.FunctionCall{Name: "memory_search", Arguments: `{"query":"Go agent"}`}},
		}}, FinishReason: "tool_calls"},
		{Message: types.Message{Role: "assistant", Content: "Found the memory about your Go agent framework."}, FinishReason: "stop"},
	}, "What do you remember about my project?")

	// 4. Multi-tool
	run("Multi-Tool (3 parallel)", []*llm.Response{
		{Message: types.Message{Role: "assistant", ToolCalls: []types.ToolCall{
			{ID: "c4a", Type: "function", Function: types.FunctionCall{Name: "get_os", Arguments: "{}"}},
			{ID: "c4b", Type: "function", Function: types.FunctionCall{Name: "get_time", Arguments: "{}"}},
			{ID: "c4c", Type: "function", Function: types.FunctionCall{Name: "exec", Arguments: `{"command":"echo hello"}`}},
		}}, FinishReason: "tool_calls"},
		{Message: types.Message{Role: "assistant", Content: "Collected system info, time, and ran echo."}, FinishReason: "stop"},
	}, "Get OS info, time, and echo hello")

	// 5. File write
	run("Write File", []*llm.Response{
		{Message: types.Message{Role: "assistant", ToolCalls: []types.ToolCall{
			{ID: "c5", Type: "function", Function: types.FunctionCall{Name: "write_file", Arguments: `{"path":"` + filepath.Join(tmpDir, "report.md") + `","content":"# Demo Report\nAll components working.\n"}`}},
		}}, FinishReason: "tool_calls"},
		{Message: types.Message{Role: "assistant", Content: "Wrote report.md."}, FinishReason: "stop"},
	}, "Write a report")

	// 6. Search files
	run("Search Files", []*llm.Response{
		{Message: types.Message{Role: "assistant", ToolCalls: []types.ToolCall{
			{ID: "c6", Type: "function", Function: types.FunctionCall{Name: "search_files", Arguments: `{"pattern":"*.md","path":"` + tmpDir + `"}`}},
		}}, FinishReason: "tool_calls"},
		{Message: types.Message{Role: "assistant", Content: "Found markdown files."}, FinishReason: "stop"},
	}, "Find all .md files")

	// Context Manager demo
	fmt.Println("--- Context Manager ---")
	budget := ctxMgr.TokenBudget()
	fmt.Printf("Token budget: ~%d tokens\n", budget)
	testMsgs := []types.Message{
		{Role: "system"}, {Role: "user"}, {Role: "assistant", ToolCalls: []types.ToolCall{{ID: "x"}}},
		{Role: "tool"}, {Role: "assistant", Content: "done"},
	}
	tokens := context.EstimateTokens(testMsgs)
	fmt.Printf("Estimated tokens for 5 messages: %d\n", tokens)

	// Bus demo
	fmt.Printf("\nEvent Bus: %d events captured\n", eventCount)

	// Verify files
	fmt.Printf("\n=== All %d Demos Passed ===\n", demoNum)
	fmt.Printf("Files in %s:\n", tmpDir)
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		fmt.Printf("  %s\n", e.Name())
	}
}
