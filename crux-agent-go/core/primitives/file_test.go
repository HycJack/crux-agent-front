package primitives

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hermes-go/core/types"
)

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3"), 0644)

	fp := NewFile()
	result := fp.Handle(types.ToolCall{
		ID:       "c1",
		Function: types.FunctionCall{Name: "read_file", Arguments: `{"path":"` + path + `"}`},
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !contains(result.Content, "line1") {
		t.Errorf("missing line1 in: %s", result.Content)
	}
	if !contains(result.Content, "Total lines: 3") {
		t.Errorf("missing total lines in: %s", result.Content)
	}
}

func TestReadFileWithOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("a\nb\nc\nd\ne\n"), 0644)

	fp := NewFile()
	result := fp.Handle(types.ToolCall{
		ID: "c1",
		Function: types.FunctionCall{
			Name:      "read_file",
			Arguments: `{"path":"` + path + `","offset":3,"limit":2}`,
		},
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !contains(result.Content, "3|c") {
		t.Errorf("expected line 3, got: %s", result.Content)
	}
	if contains(result.Content, "1|a") {
		t.Errorf("should not contain line 1: %s", result.Content)
	}
}

func TestReadFileNotFound(t *testing.T) {
	fp := NewFile()
	result := fp.Handle(types.ToolCall{
		ID:       "c1",
		Function: types.FunctionCall{Name: "read_file", Arguments: `{"path":"/nonexistent/file.txt"}`},
	})

	if !result.IsError {
		t.Fatal("expected error for missing file")
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "new.txt")

	fp := NewFile()
	result := fp.Handle(types.ToolCall{
		ID: "c1",
		Function: types.FunctionCall{
			Name:      "write_file",
			Arguments: `{"path":"` + path + `","content":"hello world"}`,
		},
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("content mismatch: got %s", string(data))
	}
}

func TestPatchFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world\nfoo bar\n"), 0644)

	fp := NewFile()
	result := fp.Handle(types.ToolCall{
		ID: "c1",
		Function: types.FunctionCall{
			Name:      "patch_file",
			Arguments: `{"path":"` + path + `","old_string":"world","new_string":"golang"}`,
		},
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	data, _ := os.ReadFile(path)
	if !contains(string(data), "hello golang") {
		t.Errorf("patch failed: got %s", string(data))
	}
}

func TestPatchFileNotFound(t *testing.T) {
	fp := NewFile()
	result := fp.Handle(types.ToolCall{
		ID: "c1",
		Function: types.FunctionCall{
			Name:      "patch_file",
			Arguments: `{"path":"/nonexistent","old_string":"a","new_string":"b"}`,
		},
	})

	if !result.IsError {
		t.Fatal("expected error")
	}
}

func TestPatchFileStringNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	fp := NewFile()
	result := fp.Handle(types.ToolCall{
		ID: "c1",
		Function: types.FunctionCall{
			Name:      "patch_file",
			Arguments: `{"path":"` + path + `","old_string":"xyz","new_string":"abc"}`,
		},
	})

	if !result.IsError {
		t.Fatal("expected error for missing string")
	}
}

func TestSearchContent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc hello() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\nfunc world() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("not a go file"), 0644)

	fp := NewFile()
	result := fp.Handle(types.ToolCall{
		ID: "c1",
		Function: types.FunctionCall{
			Name:      "search_content",
			Arguments: `{"pattern":"func ","path":"` + dir + `","file_glob":"*.go"}`,
		},
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !contains(result.Content, "hello") {
		t.Errorf("missing hello: %s", result.Content)
	}
	if !contains(result.Content, "world") {
		t.Errorf("missing world: %s", result.Content)
	}
	if contains(result.Content, "c.txt") {
		t.Errorf("should not include .txt files: %s", result.Content)
	}
}

func TestSearchFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), nil, 0644)
	os.WriteFile(filepath.Join(dir, "test.go"), nil, 0644)
	os.WriteFile(filepath.Join(dir, "readme.md"), nil, 0644)

	fp := NewFile()
	result := fp.Handle(types.ToolCall{
		ID: "c1",
		Function: types.FunctionCall{
			Name:      "search_files",
			Arguments: `{"pattern":"*.go","path":"` + dir + `"}`,
		},
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !contains(result.Content, "main.go") {
		t.Errorf("missing main.go: %s", result.Content)
	}
	if !contains(result.Content, "test.go") {
		t.Errorf("missing test.go: %s", result.Content)
	}
	if contains(result.Content, "readme.md") {
		t.Errorf("should not include .md: %s", result.Content)
	}
}

func TestToolSchemas(t *testing.T) {
	fp := NewFile()
	schemas := fp.ToolSchemas()
	if len(schemas) != 5 {
		t.Fatalf("expected 5 schemas, got %d", len(schemas))
	}
	names := map[string]bool{}
	for _, s := range schemas {
		names[s.Name] = true
	}
	for _, expected := range []string{"read_file", "write_file", "patch_file", "search_content", "search_files"} {
		if !names[expected] {
			t.Errorf("missing schema: %s", expected)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
