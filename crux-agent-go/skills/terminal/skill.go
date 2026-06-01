package terminal

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/hermes-go/core/types"
)

// Skill executes shell commands.
type Skill struct {
	WorkDir     string
	Timeout     time.Duration
	MaxOutput   int
	AllowedCmds []string // empty = all allowed
}

func New() *Skill {
	return &Skill{
		Timeout:   30 * time.Second,
		MaxOutput: 32768,
	}
}

func (s *Skill) Name() string { return "terminal" }

func (s *Skill) Init(ctx interface{}) error { return nil }

func (s *Skill) Shutdown() error { return nil }

func (s *Skill) ToolSchemas() []types.ToolSchema {
	return []types.ToolSchema{
		{
			Name:        "exec",
			Description: "Execute a shell command and return output. Has 30s timeout.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string", "description": "Shell command to execute"},
				},
				"required": []string{"command"},
			},
		},
	}
}

func (s *Skill) Capabilities() interface{} {
	return nil
}

func (s *Skill) Handle(call types.ToolCall) types.ToolResult {
	var args map[string]any
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return types.ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Error: invalid arguments: %v", err), IsError: true}
	}

	command, _ := args["command"].(string)
	if command == "" {
		return types.ToolResult{ToolCallID: call.ID, Content: "Error: command is required", IsError: true}
	}

	// Check allowed commands
	if len(s.AllowedCmds) > 0 {
		allowed := false
		firstWord := strings.Fields(command)[0]
		for _, cmd := range s.AllowedCmds {
			if firstWord == cmd {
				allowed = true
				break
			}
		}
		if !allowed {
			return types.ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Error: command %q is not allowed", firstWord), IsError: true}
		}
	}

	// Execute with timeout
	ctx, cancel := context.WithTimeout(context.Background(), s.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	if s.WorkDir != "" {
		cmd.Dir = s.WorkDir
	}

	out, err := cmd.CombinedOutput()
	output := string(out)

	// Truncate if too long
	if len(output) > s.MaxOutput {
		output = output[:s.MaxOutput] + "\n...(truncated)"
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return types.ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Error: command timed out after %s\n%s", s.Timeout, output), IsError: true}
		}
		return types.ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("%s\nError: %v", output, err), IsError: true}
	}

	if output == "" {
		output = "(no output)"
	}
	return types.ToolResult{ToolCallID: call.ID, Content: output}
}
