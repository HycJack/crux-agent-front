package memory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hermes-go/core/types"
)

func TestMemorySaveAndSearch(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	s.Init(nil)

	// Save
	call := types.ToolCall{
		ID:       "c1",
		Function: types.FunctionCall{Name: "memory_save", Arguments: `{"key":"proj","value":"Building Hermes in Go","category":"memory"}`},
	}
	result := s.Handle(call)
	if result.IsError {
		t.Fatalf("save failed: %s", result.Content)
	}

	// Search
	call = types.ToolCall{
		ID:       "c2",
		Function: types.FunctionCall{Name: "memory_search", Arguments: `{"query":"Hermes"}`},
	}
	result = s.Handle(call)
	if result.IsError {
		t.Fatalf("search failed: %s", result.Content)
	}
	if result.Content == "No matching memories found." {
		t.Fatal("expected to find memory")
	}
}

func TestMemoryList(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	s.Init(nil)

	s.Handle(types.ToolCall{ID: "c1", Function: types.FunctionCall{Name: "memory_save", Arguments: `{"key":"a","value":"val1","category":"test"}`}})
	s.Handle(types.ToolCall{ID: "c2", Function: types.FunctionCall{Name: "memory_save", Arguments: `{"key":"b","value":"val2","category":"test"}`}})

	call := types.ToolCall{ID: "c3", Function: types.FunctionCall{Name: "memory_list", Arguments: `{}`}}
	result := s.Handle(call)
	if result.IsError {
		t.Fatalf("list failed: %s", result.Content)
	}
}

func TestMemoryDelete(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	s.Init(nil)

	s.Handle(types.ToolCall{ID: "c1", Function: types.FunctionCall{Name: "memory_save", Arguments: `{"key":"temp","value":"to be deleted"}`}})

	call := types.ToolCall{ID: "c2", Function: types.FunctionCall{Name: "memory_delete", Arguments: `{"key":"temp"}`}}
	result := s.Handle(call)
	if result.IsError {
		t.Fatalf("delete failed: %s", result.Content)
	}
}

func TestMemoryPersistence(t *testing.T) {
	dir := t.TempDir()
	s1 := New(dir)
	s1.Init(nil)

	s1.Handle(types.ToolCall{ID: "c1", Function: types.FunctionCall{Name: "memory_save", Arguments: `{"key":"persist","value":"this should survive"}`}})
	s1.Shutdown()

	s2 := New(dir)
	s2.Init(nil)

	call := types.ToolCall{ID: "c2", Function: types.FunctionCall{Name: "memory_search", Arguments: `{"query":"survive"}`}}
	result := s2.Handle(call)
	if result.Content == "No matching memories found." {
		t.Fatal("expected memory to persist across instances")
	}
}

func TestGetMemoryText(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	s.Init(nil)

	s.Handle(types.ToolCall{ID: "c1", Function: types.FunctionCall{Name: "memory_save", Arguments: `{"key":"name","value":"Hermes","category":"user"}`}})
	s.Handle(types.ToolCall{ID: "c2", Function: types.FunctionCall{Name: "memory_save", Arguments: `{"key":"lang","value":"Go","category":"env"}`}})

	text := s.GetMemoryText()
	if text == "" {
		t.Fatal("expected non-empty memory text")
	}
}

func TestMemoryUnknownTool(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	s.Init(nil)

	call := types.ToolCall{ID: "c1", Function: types.FunctionCall{Name: "unknown_tool", Arguments: `{}`}}
	result := s.Handle(call)
	if !result.IsError {
		t.Fatal("expected error for unknown tool")
	}
}

func TestMemorySearchNoMatch(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	s.Init(nil)

	call := types.ToolCall{ID: "c1", Function: types.FunctionCall{Name: "memory_search", Arguments: `{"query":"nonexistent"}`}}
	result := s.Handle(call)
	if result.Content != "No matching memories found." {
		t.Fatalf("expected no match message, got: %s", result.Content)
	}
}

func TestMemoryDirCreation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "memory")
	s := New(dir)
	if err := s.Init(nil); err != nil {
		t.Fatalf("Init should create dirs: %v", err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("directory should exist")
	}
}

func TestToolSchemas(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	schemas := s.ToolSchemas()
	if len(schemas) != 4 {
		t.Fatalf("expected 4 tool schemas, got %d", len(schemas))
	}
}

func TestMemoryName(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if s.Name() != "memory" {
		t.Fatalf("expected 'memory', got '%s'", s.Name())
	}
}
