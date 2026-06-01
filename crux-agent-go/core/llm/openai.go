package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/hermes-go/core/types"
)

// Provider is the interface all LLM backends must implement.
type Provider interface {
	Complete(messages []types.Message, tools []types.ToolSchema) (*Response, error)
	Stream(messages []types.Message, tools []types.ToolSchema) (*StreamResponse, error)
	Model() string
}

// StreamResponse holds a streaming LLM response.
type StreamResponse struct {
	Chan      <-chan StreamChunk
	Cancel    func()
}

// StreamChunk is a single chunk from a streaming response.
type StreamChunk struct {
	Delta        string
	ToolCalls    []types.ToolCall
	FinishReason string
	Usage        *Usage
	Error        error
	Done         bool
}

// Response is the parsed LLM response.
type Response struct {
	Message      types.Message
	Usage        Usage
	FinishReason string
}

// Usage tracks token consumption.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAI implements Provider for any OpenAI-compatible API.
type OpenAI struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
	retries int
}

func NewOpenAI(apiKey, baseURL, model string) *OpenAI {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAI{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 180 * time.Second},
		retries: 3,
	}
}

func (p *OpenAI) Model() string { return p.model }

func (p *OpenAI) Complete(messages []types.Message, tools []types.ToolSchema) (*Response, error) {
	body := map[string]any{
		"model":       p.model,
		"messages":    messages,
		"max_tokens":  4096,
		"temperature": 0.7,
	}
	if len(tools) > 0 {
		openaiTools := make([]map[string]any, len(tools))
		for i, tool := range tools {
			openaiTools[i] = map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        tool.Name,
					"description": tool.Description,
					"parameters":  tool.Parameters,
				},
			}
		}
		body["tools"] = openaiTools
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= p.retries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second)
		}

		resp, err := p.doRequest(data)
		if err != nil {
			lastErr = err
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("retries exhausted: %w", lastErr)
}

func (p *OpenAI) doRequest(data []byte) (*Response, error) {
	req, err := http.NewRequest("POST", p.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Role      string        `json:"role"`
				Content   string        `json:"content"`
				ToolCalls []types.ToolCall `json:"tool_calls,omitempty"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := result.Choices[0]
	// Ensure tool call type is set
	for i := range choice.Message.ToolCalls {
		if choice.Message.ToolCalls[i].Type == "" {
			choice.Message.ToolCalls[i].Type = "function"
		}
	}
	return &Response{
		Message: types.Message{
			Role:      choice.Message.Role,
			Content:   choice.Message.Content,
			ToolCalls: choice.Message.ToolCalls,
		},
		Usage:        result.Usage,
		FinishReason: choice.FinishReason,
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// Stream implements streaming chat completion via SSE.
func (p *OpenAI) Stream(messages []types.Message, tools []types.ToolSchema) (*StreamResponse, error) {
	// Log message summary for debugging
	msgSummary := make([]string, len(messages))
	for i, m := range messages {
		contentLen := len(m.Content)
		if m.Role == "tool" {
			msgSummary[i] = fmt.Sprintf("tool(id=%s,content=%d)", m.ToolCallID, contentLen)
		} else if len(m.ToolCalls) > 0 {
			tcNames := make([]string, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				tcNames[j] = tc.Function.Name
			}
			msgSummary[i] = fmt.Sprintf("%s(tool_calls=%v)", m.Role, tcNames)
		} else {
			msgSummary[i] = fmt.Sprintf("%s(%d)", m.Role, contentLen)
		}
	}
	log.Printf("[LLM] Stream request: model=%s, messages=%d, tools=%d, msgs=%v", p.model, len(messages), len(tools), msgSummary)

	body := map[string]any{
		"model":       p.model,
		"messages":    messages,
		"max_tokens":  4096,
		"temperature": 0.7,
		"stream":      true,
	}
	if len(tools) > 0 {
		openaiTools := make([]map[string]any, len(tools))
		for i, tool := range tools {
			openaiTools[i] = map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        tool.Name,
					"description": tool.Description,
					"parameters":  tool.Parameters,
				},
			}
		}
		body["tools"] = openaiTools
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", p.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("API error %d: %s", resp.StatusCode, truncate(string(respBody), 500))
		log.Printf("[LLM] %s", errMsg)
		return nil, fmt.Errorf(errMsg)
	}

	ch := make(chan StreamChunk, 64)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer resp.Body.Close()
		defer close(ch)
		p.streamLoop(ctx, resp.Body, ch)
	}()

	return &StreamResponse{Chan: ch, Cancel: cancel}, nil
}

func (p *OpenAI) streamLoop(ctx context.Context, body io.Reader, ch chan<- StreamChunk) {
	scanner := bufio.NewScanner(body)
	// Increase buffer for large chunks
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024)

	// Accumulate tool calls across chunks
	var accToolCalls []types.ToolCall

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- StreamChunk{Error: ctx.Err(), Done: true}
			return
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			ch <- StreamChunk{ToolCalls: accToolCalls, Done: true}
			return
		}

		var evt struct {
			Choices []struct {
				Delta struct {
					Content   string            `json:"content"`
					ToolCalls []types.ToolCall  `json:"tool_calls,omitempty"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *Usage `json:"usage,omitempty"`
		}

		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			ch <- StreamChunk{Error: fmt.Errorf("parse chunk: %w", err), Done: true}
			return
		}

		if len(evt.Choices) == 0 {
			continue
		}

		choice := evt.Choices[0]

		// Accumulate tool calls (streaming sends partial tool calls across chunks)
		if len(choice.Delta.ToolCalls) > 0 {
			for _, tc := range choice.Delta.ToolCalls {
				if tc.Type == "" {
					tc.Type = "function"
				}
				// If ID is present, this is a new tool call or a match by ID
				// If ID is empty, this is an argument continuation for the last tool call
				if tc.ID == "" {
					// Merge into the last tool call (argument continuation)
					if len(accToolCalls) > 0 {
						last := len(accToolCalls) - 1
						if tc.Function.Arguments != "" {
							accToolCalls[last].Function.Arguments += tc.Function.Arguments
						}
						if tc.Function.Name != "" {
							accToolCalls[last].Function.Name = tc.Function.Name
						}
					}
					continue
				}
				// ID present: find existing or append new
				found := false
				for i := range accToolCalls {
					if accToolCalls[i].ID == tc.ID {
						if tc.Function.Arguments != "" {
							accToolCalls[i].Function.Arguments += tc.Function.Arguments
						}
						if tc.Function.Name != "" {
							accToolCalls[i].Function.Name = tc.Function.Name
						}
						found = true
						break
					}
				}
				if !found {
					accToolCalls = append(accToolCalls, tc)
				}
			}
		}

		chunk := StreamChunk{
			Delta:        choice.Delta.Content,
			FinishReason: choice.FinishReason,
			Usage:        evt.Usage,
		}

		if choice.FinishReason != "" {
			chunk.ToolCalls = accToolCalls
			chunk.Done = true
		}

		ch <- chunk
	}

	if err := scanner.Err(); err != nil {
		ch <- StreamChunk{Error: fmt.Errorf("scan: %w", err), Done: true}
	}
}
