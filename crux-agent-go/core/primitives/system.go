package primitives

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/hermes-go/core/types"
)

// SystemPrimitives provides 4 system info operations.
type SystemPrimitives struct {
	cwd string
}

func NewSystem() *SystemPrimitives {
	cwd, _ := os.Getwd()
	return &SystemPrimitives{cwd: cwd}
}

func (sp *SystemPrimitives) ToolSchemas() []types.ToolSchema {
	return []types.ToolSchema{
		{Name: "get_time", Description: "Get current time (UTC, local, Unix timestamp)."},
		{Name: "get_os", Description: "Get OS, architecture, hostname, CPU count."},
		{Name: "get_cwd", Description: "Get current working directory."},
		{
			Name:        "get_env",
			Description: "Get an environment variable value.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key": map[string]any{"type": "string", "description": "Environment variable name"},
				},
				"required": []string{"key"},
			},
		},
	}
}

func (sp *SystemPrimitives) Handle(call types.ToolCall) types.ToolResult {
	var content string

	switch call.Function.Name {
	case "get_time":
		now := time.Now()
		content = fmt.Sprintf("UTC: %s\nLocal: %s\nUnix: %d\nTimezone: %s",
			now.UTC().Format(time.RFC3339),
			now.Format("2006-01-02 15:04:05"),
			now.Unix(),
			now.Location())
	case "get_os":
		hostname, _ := os.Hostname()
		content = fmt.Sprintf("OS: %s\nArch: %s\nHostname: %s\nCPUs: %d",
			runtime.GOOS, runtime.GOARCH, hostname, runtime.NumCPU())
	case "get_cwd":
		content = sp.cwd
	case "get_env":
		var args struct {
			Key string `json:"key"`
		}
		json.Unmarshal([]byte(call.Function.Arguments), &args)
		content = os.Getenv(args.Key)
		if content == "" {
			content = fmt.Sprintf("(not set: %s)", args.Key)
		}
	default:
		return types.ToolResult{
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("Unknown tool: %s", call.Function.Name),
			IsError:    true,
		}
	}

	return types.ToolResult{ToolCallID: call.ID, Content: content}
}
