package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hermes-go/core/types"
)

func TestOpenAIComplete(t *testing.T) {
	// Mock OpenAI API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("unexpected auth: %s", r.Header.Get("Authorization"))
		}

		// Parse request
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["model"] != "test-model" {
			t.Errorf("unexpected model: %v", req["model"])
		}

		// Return mock response
		resp := map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": "Hello! I'm a test response.",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOpenAI("sk-test", server.URL, "test-model")

	messages := []types.Message{
		{Role: "user", Content: "Hello"},
	}

	resp, err := provider.Complete(messages, nil)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	if resp.Message.Content != "Hello! I'm a test response." {
		t.Errorf("content: got %s", resp.Message.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("usage: got %d", resp.Usage.TotalTokens)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("finish_reason: got %s", resp.FinishReason)
	}
}

func TestOpenAIWithTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)

		// Verify tools were sent
		tools, ok := req["tools"].([]any)
		if !ok || len(tools) == 0 {
			t.Fatal("expected tools in request")
		}

		// Return tool call response
		resp := map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role": "assistant",
					"tool_calls": []map[string]any{{
						"id":   "call-123",
						"type": "function",
						"function": map[string]any{
							"name":      "read_file",
							"arguments": `{"path":"/tmp/test.txt"}`,
						},
					}},
				},
				"finish_reason": "tool_calls",
			}},
			"usage": map[string]any{
				"prompt_tokens":     20,
				"completion_tokens": 10,
				"total_tokens":      30,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOpenAI("sk-test", server.URL, "test-model")

	tools := []types.ToolSchema{
		{Name: "read_file", Description: "Read a file"},
	}
	messages := []types.Message{
		{Role: "user", Content: "Read /tmp/test.txt"},
	}

	resp, err := provider.Complete(messages, tools)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.Message.ToolCalls))
	}
	tc := resp.Message.ToolCalls[0]
	if tc.ID != "call-123" {
		t.Errorf("tool call ID: got %s", tc.ID)
	}
	if tc.Function.Name != "read_file" {
		t.Errorf("tool name: got %s", tc.Function.Name)
	}
}

func TestOpenAIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "invalid request"}`))
	}))
	defer server.Close()

	provider := NewOpenAI("sk-test", server.URL, "test-model")
	_, err := provider.Complete([]types.Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
}

func TestOpenAIModel(t *testing.T) {
	p := NewOpenAI("sk-test", "http://localhost", "gpt-4o")
	if p.Model() != "gpt-4o" {
		t.Errorf("model: got %s", p.Model())
	}
}
