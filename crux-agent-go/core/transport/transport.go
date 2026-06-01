package transport

import (
	"context"
	"fmt"
	"time"

	"github.com/hermes-go/core/types"
)

// Transport is the interface for agent-to-agent communication.
type Transport interface {
	// Discover gets the target agent's capabilities.
	Discover(target Target) (*AgentCard, error)
	// SendTask sends a task and waits for the result.
	SendTask(ctx context.Context, target Target, task *Task) (*TaskResult, error)
	// SendTaskStream sends a task and streams events back.
	SendTaskStream(ctx context.Context, target Target, task *Task) (<-chan TaskEvent, error)
	// CancelTask cancels a running task.
	CancelTask(ctx context.Context, target Target, taskID string) error
}

// Target identifies an agent to communicate with.
type Target struct {
	ID       string
	Address  string // "memory:" for in-process, "unix:" for local, "https:" for remote
	Protocol string // "a2a", "acp", "local", "memory"
}

// Task is a unit of work sent to another agent.
type Task struct {
	ID       string
	Message  string
	Config   TaskConfig
	Metadata map[string]any
}

type TaskConfig struct {
	Timeout   time.Duration
	MaxTokens int
}

// TaskResult is the outcome of a delegated task.
type TaskResult struct {
	ID       string
	Status   string // "completed", "failed", "canceled"
	Messages []types.Message
	Error    string
}

// TaskEvent is a streaming event from a task.
type TaskEvent struct {
	Type    string // "status", "message", "error"
	TaskID  string
	Content string
}

// AgentCard describes an agent's capabilities.
type AgentCard struct {
	Name        string
	Description string
	URL         string
	Skills      []string
	Capabilities map[string]bool
}

// InProcess is a transport for agents running in the same process.
type InProcess struct {
	handler func(task *Task) (*TaskResult, error)
}

func NewInProcess(handler func(task *Task) (*TaskResult, error)) *InProcess {
	return &InProcess{handler: handler}
}

func (t *InProcess) Discover(target Target) (*AgentCard, error) {
	return &AgentCard{
		Name:        target.ID,
		Description: "In-process agent",
		Capabilities: map[string]bool{"streaming": false},
	}, nil
}

func (t *InProcess) SendTask(ctx context.Context, target Target, task *Task) (*TaskResult, error) {
	if t.handler == nil {
		return nil, fmt.Errorf("no handler registered for %s", target.ID)
	}
	return t.handler(task)
}

func (t *InProcess) SendTaskStream(ctx context.Context, target Target, task *Task) (<-chan TaskEvent, error) {
	return nil, fmt.Errorf("streaming not supported for in-process transport")
}

func (t *InProcess) CancelTask(ctx context.Context, target Target, taskID string) error {
	return fmt.Errorf("cancel not supported for in-process transport")
}
