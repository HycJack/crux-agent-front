package transport

import (
	"context"
	"testing"

	"github.com/hermes-go/core/types"
)

func TestInProcessTransport(t *testing.T) {
	handler := func(task *Task) (*TaskResult, error) {
		return &TaskResult{
			ID:       task.ID,
			Status:   "completed",
			Messages: []types.Message{{Role: "assistant", Content: "done: " + task.Message}},
		}, nil
	}

	tr := NewInProcess(handler)
	target := Target{ID: "test-agent", Protocol: "memory"}

	// Discover
	card, err := tr.Discover(target)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if card.Name != "test-agent" {
		t.Errorf("name: %s", card.Name)
	}

	// SendTask
	result, err := tr.SendTask(context.Background(), target, &Task{
		ID:      "t1",
		Message: "hello",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("status: %s", result.Status)
	}
	if result.Messages[0].Content != "done: hello" {
		t.Errorf("content: %s", result.Messages[0].Content)
	}
}
