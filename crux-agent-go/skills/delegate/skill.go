package delegate

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hermes-go/core/registry"
	"github.com/hermes-go/core/transport"
	"github.com/hermes-go/core/types"
)

// Skill enables one agent to delegate tasks to other agents.
type Skill struct {
	registry *registry.Registry
}

func New(reg *registry.Registry) *Skill {
	return &Skill{registry: reg}
}

func (s *Skill) Name() string { return "delegate" }

func (s *Skill) Capabilities() struct{ Tools []types.ToolSchema } {
	return struct{ Tools []types.ToolSchema }{
		Tools: []types.ToolSchema{
			{
				Name:        "delegate_task",
				Description: "Delegate a task to another agent. The agent executes the task and returns the result.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"agent":   map[string]any{"type": "string", "description": "Target agent name or ID"},
						"task":    map[string]any{"type": "string", "description": "Task description"},
						"timeout": map[string]any{"type": "integer", "description": "Timeout in seconds (default 60)"},
					},
					"required": []string{"agent", "task"},
				},
			},
			{
				Name:        "discover_agents",
				Description: "List available agents and their capabilities.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string", "description": "Search query (optional)"},
					},
				},
			},
		},
	}
}

func (s *Skill) Handle(call types.ToolCall) types.ToolResult {
	var args map[string]any
	json.Unmarshal([]byte(call.Function.Arguments), &args)

	var content string
	var err error

	switch call.Function.Name {
	case "delegate_task":
		content, err = s.delegateTask(args)

	case "discover_agents":
		content, err = s.discoverAgents(args)

	default:
		err = fmt.Errorf("unknown tool: %s", call.Function.Name)
	}

	result := types.ToolResult{ToolCallID: call.ID}
	if err != nil {
		result.Content = fmt.Sprintf("Error: %v", err)
		result.IsError = true
	} else {
		result.Content = content
	}
	return result
}

func (s *Skill) delegateTask(args map[string]any) (string, error) {
	agentID, _ := args["agent"].(string)
	task, _ := args["task"].(string)
	timeout := 60
	if v, ok := args["timeout"].(float64); ok {
		timeout = int(v)
	}

	if agentID == "" || task == "" {
		return "", fmt.Errorf("agent and task are required")
	}

	// Look up agent in registry
	agent, err := s.registry.Get(agentID)
	if err != nil {
		return "", err
	}

	// Send task via the agent's transport
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	taskObj := &transport.Task{
		ID:      fmt.Sprintf("task-%d", time.Now().UnixNano()),
		Message: task,
		Config:  transport.TaskConfig{Timeout: time.Duration(timeout) * time.Second},
	}

	target := transport.Target{
		ID:       agentID,
		Address:  agent.Location,
		Protocol: "memory",
	}

	result, err := agent.Transport.SendTask(ctx, target, taskObj)
	if err != nil {
		return "", fmt.Errorf("delegate failed: %w", err)
	}

	// Format result
	var response string
	switch result.Status {
	case "completed":
		for _, msg := range result.Messages {
			if msg.Role == "assistant" && msg.Content != "" {
				response += msg.Content + "\n"
			}
		}
		if response == "" {
			response = "Task completed."
		}
	case "failed":
		response = fmt.Sprintf("Task failed: %s", result.Error)
	default:
		response = fmt.Sprintf("Task status: %s", result.Status)
	}

	return response, nil
}

func (s *Skill) discoverAgents(args map[string]any) (string, error) {
	query, _ := args["query"].(string)

	var agents []*registry.RegisteredAgent
	if query != "" {
		agents = s.registry.Search(query)
	} else {
		agents = s.registry.List()
	}

	if len(agents) == 0 {
		return "No agents available.", nil
	}

	var result string
	for _, a := range agents {
		result += fmt.Sprintf("- %s: %s (status: %s, skills: %v)\n",
			a.Card.Name, a.Card.Description, a.Status, a.Card.Skills)
	}
	return result, nil
}
