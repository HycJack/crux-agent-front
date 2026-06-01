package tools

import (
	"testing"

	"github.com/hermes-go/core/agent"
)

func TestRegisterAndGet(t *testing.T) {
	Clear()
	tool := agent.AgentTool{
		Name:        "test_tool",
		Description: "A test tool",
		Execute: func(call agent.AgentToolCall) agent.AgentToolResult {
			return agent.AgentToolResult{ToolCallID: call.ID, Content: "ok"}
		},
	}
	Register("test_tool", tool, "builtin", "test")

	entry, ok := Get("test_tool")
	if !ok {
		t.Fatal("expected to find test_tool")
	}
	if entry.Source != "builtin" {
		t.Fatalf("expected builtin, got %s", entry.Source)
	}
}

func TestList(t *testing.T) {
	Clear()
	Register("a", agent.AgentTool{Name: "a"}, "builtin", "file")
	Register("b", agent.AgentTool{Name: "b"}, "skill", "terminal")

	entries := List()
	if len(entries) != 2 {
		t.Fatalf("expected 2, got %d", len(entries))
	}
}

func TestByCategory(t *testing.T) {
	Clear()
	Register("read_file", agent.AgentTool{Name: "read_file"}, "builtin", "file")
	Register("exec", agent.AgentTool{Name: "exec"}, "builtin", "terminal")
	Register("write_file", agent.AgentTool{Name: "write_file"}, "builtin", "file")

	fileTools := ByCategory("file")
	if len(fileTools) != 2 {
		t.Fatalf("expected 2 file tools, got %d", len(fileTools))
	}
}

func TestBySource(t *testing.T) {
	Clear()
	Register("a", agent.AgentTool{Name: "a"}, "builtin", "test")
	Register("b", agent.AgentTool{Name: "b"}, "plugin", "test")

	builtin := BySource("builtin")
	if len(builtin) != 1 {
		t.Fatalf("expected 1 builtin, got %d", len(builtin))
	}
}

func TestAgentTools(t *testing.T) {
	Clear()
	Register("a", agent.AgentTool{Name: "a"}, "builtin", "test")
	Register("b", agent.AgentTool{Name: "b"}, "builtin", "test")

	tools := AgentTools()
	if len(tools) != 2 {
		t.Fatalf("expected 2, got %d", len(tools))
	}
}

func TestString(t *testing.T) {
	Clear()
	Register("a", agent.AgentTool{Name: "a"}, "builtin", "file")
	s := String()
	if s == "" {
		t.Fatal("expected non-empty string")
	}
}
