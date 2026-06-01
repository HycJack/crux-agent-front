package types

import (
	"encoding/json"
	"testing"
)

func TestMessage(t *testing.T) {
	msg := Message{
		Role:    "user",
		Content: "hello",
	}
	if msg.Role != "user" {
		t.Fatalf("expected role user, got %s", msg.Role)
	}

	// JSON round-trip
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Message
	json.Unmarshal(data, &decoded)
	if decoded.Content != "hello" {
		t.Fatalf("round-trip failed: got %s", decoded.Content)
	}
}

func TestToolCall(t *testing.T) {
	tc := ToolCall{
		ID:   "call-1",
		Type: "function",
		Function: FunctionCall{
			Name:      "read_file",
			Arguments: `{"path":"/tmp/test.txt"}`,
		},
	}
	if tc.Function.Name != "read_file" {
		t.Fatalf("expected read_file, got %s", tc.Function.Name)
	}

	var args map[string]string
	json.Unmarshal([]byte(tc.Function.Arguments), &args)
	if args["path"] != "/tmp/test.txt" {
		t.Fatalf("expected /tmp/test.txt, got %s", args["path"])
	}
}

func TestToolResult(t *testing.T) {
	tr := ToolResult{
		ToolCallID: "call-1",
		Content:    "file content here",
		IsError:    false,
	}
	if tr.IsError {
		t.Fatal("expected not error")
	}

	trErr := ToolResult{
		ToolCallID: "call-2",
		Content:    "file not found",
		IsError:    true,
	}
	if !trErr.IsError {
		t.Fatal("expected error")
	}
}

func TestToolSchema(t *testing.T) {
	ts := ToolSchema{
		Name:        "exec",
		Description: "Execute a command",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]string{"type": "string"},
			},
			"required": []string{"command"},
		},
	}
	if ts.Name != "exec" {
		t.Fatalf("expected exec, got %s", ts.Name)
	}
	if ts.Parameters == nil {
		t.Fatal("parameters should not be nil")
	}
}

func TestSession(t *testing.T) {
	sess := Session{
		ID:    "s-1",
		Title: "test session",
		Messages: []Message{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hello!"},
		},
	}
	if len(sess.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(sess.Messages))
	}

	data, _ := json.Marshal(sess)
	var decoded Session
	json.Unmarshal(data, &decoded)
	if len(decoded.Messages) != 2 {
		t.Fatalf("round-trip: expected 2 messages, got %d", len(decoded.Messages))
	}
}
