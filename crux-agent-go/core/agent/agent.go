package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

type AgentMessage struct {
	Role         string          `json:"role"`
	Content      string          `json:"content,omitempty"`
	ToolCalls    []AgentToolCall `json:"tool_calls,omitempty"`
	ToolCallID   string          `json:"tool_call_id,omitempty"`
	Name         string          `json:"name,omitempty"`
	Timestamp    int64           `json:"timestamp,omitempty"`
	Summary      string          `json:"summary,omitempty"`
	TokensBefore int             `json:"tokens_before,omitempty"`
	CustomType   string          `json:"custom_type,omitempty"`
	Display      bool            `json:"display,omitempty"`
	FromID       string          `json:"from_id,omitempty"`
}

type AgentToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type AgentToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
	Details    any    `json:"details,omitempty"`
	Terminate  bool   `json:"terminate,omitempty"`
}

type AgentTool struct {
	Name          string                             `json:"name"`
	Description   string                             `json:"description"`
	Parameters    map[string]any                     `json:"parameters,omitempty"`
	Execute       func(call AgentToolCall) AgentToolResult `json:"-"`
	ExecutionMode string                             `json:"execution_mode,omitempty"`
}

type ToolExecutionMode string

const (
	ToolSequential ToolExecutionMode = "sequential"
	ToolParallel   ToolExecutionMode = "parallel"
)

type BeforeToolCallContext struct {
	ToolCall AgentToolCall
	Tool     *AgentTool
	Messages []AgentMessage
}

type AfterToolCallContext struct {
	ToolCall AgentToolCall
	Tool     *AgentTool
	Result   AgentToolResult
	Messages []AgentMessage
}

type BeforeToolCallResult struct {
	Block  bool   `json:"block,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type AfterToolCallResult struct {
	Content   *string `json:"content,omitempty"`
	IsError   *bool   `json:"is_error,omitempty"`
	Terminate *bool   `json:"terminate,omitempty"`
}

type AgentEvent struct {
	Type          string         `json:"type"`
	Message       *AgentMessage  `json:"message,omitempty"`
	Error         error          `json:"error,omitempty"`
	TextDelta     string         `json:"text_delta,omitempty"`
	ToolCallDelta *AgentToolCall `json:"toolcall_delta,omitempty"`
}

type StreamResult struct {
	Message AgentMessage
	Error   error
	Events  chan AgentEvent
}

type AgentLoopConfig struct {
	Tools          []AgentTool
	StreamFn       func(ctx context.Context, messages []AgentMessage, tools []AgentTool) (*StreamResult, error)
	MaxRounds      int
	ToolMode       ToolExecutionMode
	BeforeToolCall func(ctx BeforeToolCallContext) *BeforeToolCallResult
	AfterToolCall  func(ctx AfterToolCallContext) *AfterToolCallResult
	OnEvent        func(event AgentEvent)
}

type PendingMessage struct {
	Message AgentMessage
	Mode    string
}

type AgentState struct {
	Messages  []AgentMessage
	ToolCount atomic.Int64
}

type toolResult struct {
	msg       AgentMessage
	terminate bool
}

type Agent struct {
	mu            sync.Mutex
	state         AgentState
	config        AgentLoopConfig
	steeringQueue []PendingMessage
	followUpQueue []PendingMessage
	isRunning     bool
	cancelFunc    context.CancelFunc
}

func New(config AgentLoopConfig) *Agent {
	if config.MaxRounds == 0 {
		config.MaxRounds = 50
	}
	if config.ToolMode == "" {
		config.ToolMode = ToolSequential
	}
	return &Agent{config: config}
}

func (a *Agent) State() AgentState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return AgentState{Messages: append([]AgentMessage{}, a.state.Messages...)}
}

func (a *Agent) Messages() []AgentMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]AgentMessage{}, a.state.Messages...)
}

func (a *Agent) Steer(msg AgentMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.steeringQueue = append(a.steeringQueue, PendingMessage{Message: msg, Mode: "one-at-a-time"})
}

func (a *Agent) FollowUp(msg AgentMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.followUpQueue = append(a.followUpQueue, PendingMessage{Message: msg, Mode: "one-at-a-time"})
}

func (a *Agent) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.isRunning
}

func (a *Agent) Abort() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancelFunc != nil {
		a.cancelFunc()
	}
}

func (a *Agent) Run(prompts []AgentMessage) ([]AgentMessage, error) {
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.isRunning = true
	a.cancelFunc = cancel
	a.state.Messages = append(a.state.Messages, prompts...)
	messages := append([]AgentMessage{}, a.state.Messages...)
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.isRunning = false
		a.cancelFunc = nil
		a.mu.Unlock()
		cancel()
	}()

	a.emit(AgentEvent{Type: "agent_start"})
	defer a.emit(AgentEvent{Type: "agent_end"})

	exitedNormally := false

	for round := 0; round < a.config.MaxRounds; round++ {
		if ctx.Err() != nil {
			return messages, fmt.Errorf("aborted: %w", ctx.Err())
		}

		a.emit(AgentEvent{Type: "turn_start"})

		result, err := a.config.StreamFn(ctx, messages, a.config.Tools)
		if err != nil {
			return messages, fmt.Errorf("llm error at round %d: %w", round, err)
		}
		if result.Error != nil {
			return messages, fmt.Errorf("llm stream error: %w", result.Error)
		}

		messages = append(messages, result.Message)
		a.mu.Lock()
		a.state.Messages = append(a.state.Messages, result.Message)
		a.mu.Unlock()
		a.emit(AgentEvent{Type: "message_end", Message: &result.Message})

		if len(result.Message.ToolCalls) == 0 {
			a.emit(AgentEvent{Type: "turn_end"})
			exitedNormally = true
			break
		}

		var shouldTerminate bool
		if a.config.ToolMode == ToolParallel {
			shouldTerminate = a.executeToolsParallel(ctx, result.Message.ToolCalls, &messages)
		} else {
			shouldTerminate = a.executeToolsSequential(ctx, result.Message.ToolCalls, &messages)
		}

		a.emit(AgentEvent{Type: "turn_end"})

		if shouldTerminate {
			exitedNormally = true
			break
		}

		if steeringMsg := a.drainSteering(); steeringMsg != nil {
			messages = append(messages, *steeringMsg)
			a.mu.Lock()
			a.state.Messages = append(a.state.Messages, *steeringMsg)
			a.mu.Unlock()
		}
	}

	if exitedNormally {
		a.processFollowUps()
		return messages, nil
	}

	return messages, fmt.Errorf("max rounds (%d) exceeded", a.config.MaxRounds)
}

func (a *Agent) executeToolsSequential(ctx context.Context, calls []AgentToolCall, messages *[]AgentMessage) bool {
	for _, call := range calls {
		if ctx.Err() != nil {
			return true
		}
		res := a.executeSingleTool(ctx, call, *messages)
		*messages = append(*messages, res.msg)
		a.mu.Lock()
		a.state.Messages = append(a.state.Messages, res.msg)
		a.mu.Unlock()
		a.emit(AgentEvent{Type: "tool_execution_end", Message: &res.msg})
		if res.terminate {
			return true
		}
	}
	return false
}

func (a *Agent) executeToolsParallel(ctx context.Context, calls []AgentToolCall, messages *[]AgentMessage) bool {
	a.mu.Lock()
	msgSnapshot := append([]AgentMessage{}, a.state.Messages...)
	a.mu.Unlock()

	results := make([]toolResult, len(calls))
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, c AgentToolCall) {
			defer wg.Done()
			results[idx] = a.executeSingleTool(ctx, c, msgSnapshot)
		}(i, call)
	}
	wg.Wait()

	shouldTerminate := false
	for _, res := range results {
		*messages = append(*messages, res.msg)
		a.mu.Lock()
		a.state.Messages = append(a.state.Messages, res.msg)
		a.mu.Unlock()
		a.emit(AgentEvent{Type: "tool_execution_end", Message: &res.msg})
		if res.terminate {
			shouldTerminate = true
		}
	}
	return shouldTerminate
}

func (a *Agent) executeSingleTool(ctx context.Context, call AgentToolCall, messages []AgentMessage) toolResult {
	a.state.ToolCount.Add(1)

	var tool *AgentTool
	for i := range a.config.Tools {
		if a.config.Tools[i].Name == call.Function.Name {
			tool = &a.config.Tools[i]
			break
		}
	}

	if a.config.BeforeToolCall != nil && tool != nil {
		beforeCtx := BeforeToolCallContext{ToolCall: call, Tool: tool, Messages: messages}
		beforeResult := a.config.BeforeToolCall(beforeCtx)
		if beforeResult != nil && beforeResult.Block {
			return toolResult{
				msg: AgentMessage{
					Role: "tool", ToolCallID: call.ID,
					Content: fmt.Sprintf("Blocked: %s", beforeResult.Reason),
					Name:    call.Function.Name,
				},
			}
		}
	}

	a.emit(AgentEvent{Type: "tool_execution_start", Message: &AgentMessage{Role: "assistant", ToolCalls: []AgentToolCall{call}}})

	var result AgentToolResult
	if tool != nil && tool.Execute != nil {
		result = tool.Execute(call)
	} else {
		result = AgentToolResult{
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("Unknown tool: %s", call.Function.Name),
			IsError:    true,
		}
	}

	if a.config.AfterToolCall != nil && tool != nil {
		afterCtx := AfterToolCallContext{ToolCall: call, Tool: tool, Result: result, Messages: messages}
		afterResult := a.config.AfterToolCall(afterCtx)
		if afterResult != nil {
			if afterResult.Content != nil {
				result.Content = *afterResult.Content
			}
			if afterResult.IsError != nil {
				result.IsError = *afterResult.IsError
			}
			if afterResult.Terminate != nil && *afterResult.Terminate {
				result.Terminate = true
			}
		}
	}

	return toolResult{
		msg: AgentMessage{
			Role: "tool", ToolCallID: result.ToolCallID,
			Content: result.Content, Name: call.Function.Name,
		},
		terminate: result.Terminate,
	}
}

func (a *Agent) drainSteering() *AgentMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.steeringQueue) == 0 {
		return nil
	}
	msg := a.steeringQueue[0].Message
	a.steeringQueue = a.steeringQueue[1:]
	return &msg
}

func (a *Agent) processFollowUps() {
	a.mu.Lock()
	followUps := a.followUpQueue
	a.followUpQueue = nil
	a.mu.Unlock()

	if len(followUps) == 0 {
		return
	}
	a.mu.Lock()
	for _, fu := range followUps {
		a.state.Messages = append(a.state.Messages, fu.Message)
	}
	a.mu.Unlock()
}

func (a *Agent) emit(event AgentEvent) {
	if a.config.OnEvent != nil {
		a.config.OnEvent(event)
	}
}
