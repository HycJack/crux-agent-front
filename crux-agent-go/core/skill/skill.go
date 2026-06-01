package skill

import (
	"github.com/hermes-go/core/types"
)

// Skill is the standard interface all skills must implement.
type Skill interface {
	// Name returns the skill's identifier.
	Name() string

	// Init is called once when the skill is loaded.
	Init(ctx Context) error

	// Shutdown is called when the agent is shutting down.
	Shutdown() error

	// ToolSchemas returns the tools this skill provides.
	ToolSchemas() []types.ToolSchema

	// Handle processes a tool call.
	Handle(call types.ToolCall) types.ToolResult
}

// Context is passed to Skill.Init, giving it access to the runtime.
type Context interface {
	// WorkDir returns the agent's working directory.
	WorkDir() string
}

// Simple is a minimal Skill implementation for skills that don't need Init/Shutdown.
type Simple struct {
	Name_       string
	Tools_      []types.ToolSchema
	HandleFunc_ func(types.ToolCall) types.ToolResult
}

func (s *Simple) Name() string                { return s.Name_ }
func (s *Simple) Init(ctx Context) error       { return nil }
func (s *Simple) Shutdown() error              { return nil }
func (s *Simple) ToolSchemas() []types.ToolSchema { return s.Tools_ }
func (s *Simple) Handle(call types.ToolCall) types.ToolResult {
	if s.HandleFunc_ != nil {
		return s.HandleFunc_(call)
	}
	return types.ToolResult{ToolCallID: call.ID, Content: "not implemented", IsError: true}
}
