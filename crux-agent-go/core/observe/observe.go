package observe

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// TraceID uniquely identifies a task execution.
type TraceID string

func NewTraceID() TraceID {
	return TraceID(fmt.Sprintf("t-%d", time.Now().UnixNano()))
}

// Span represents a single operation within a trace.
type Span struct {
	TraceID   TraceID
	SpanID    string
	Name      string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	Attrs     map[string]any
	Status    string // "ok", "error"
	Error     string
}

// Tracer collects spans for distributed tracing.
type Tracer struct {
	mu     sync.Mutex
	spans  []Span
	logger *slog.Logger
}

func NewTracer() *Tracer {
	return &Tracer{logger: slog.Default()}
}

// Start begins a new span.
func (t *Tracer) Start(traceID TraceID, name string, attrs map[string]any) *Span {
	span := &Span{
		TraceID:   traceID,
		SpanID:    fmt.Sprintf("s-%d", time.Now().UnixNano()),
		Name:      name,
		StartTime: time.Now(),
		Attrs:     attrs,
	}
	return span
}

// End finishes a span and records it.
func (t *Tracer) End(span *Span) {
	span.EndTime = time.Now()
	span.Duration = span.EndTime.Sub(span.StartTime)
	t.mu.Lock()
	t.spans = append(t.spans, *span)
	t.mu.Unlock()
	t.logger.Debug("span", "trace", span.TraceID, "name", span.Name, "duration", span.Duration, "status", span.Status)
}

// Spans returns all recorded spans.
func (t *Tracer) Spans() []Span {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]Span, len(t.spans))
	copy(result, t.spans)
	return result
}

// Metrics tracks agent performance metrics.
type Metrics struct {
	mu              sync.Mutex
	LLMCalls        int
	LLMTokens       int
	LLMLatencyTotal time.Duration
	ToolCalls       int
	ToolErrors      int
	TasksStarted    int
	TasksCompleted  int
	TasksFailed     int
}

func NewMetrics() *Metrics {
	return &Metrics{}
}

func (m *Metrics) RecordLLM(tokens int, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LLMCalls++
	m.LLMTokens += tokens
	m.LLMLatencyTotal += latency
}

func (m *Metrics) RecordTool(isError bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ToolCalls++
	if isError {
		m.ToolErrors++
	}
}

func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	avgLatency := time.Duration(0)
	if m.LLMCalls > 0 {
		avgLatency = m.LLMLatencyTotal / time.Duration(m.LLMCalls)
	}
	return MetricsSnapshot{
		LLMCalls:        m.LLMCalls,
		LLMTokens:       m.LLMTokens,
		LLMAvgLatency:   avgLatency,
		ToolCalls:       m.ToolCalls,
		ToolErrors:      m.ToolErrors,
		TasksStarted:    m.TasksStarted,
		TasksCompleted:  m.TasksCompleted,
		TasksFailed:     m.TasksFailed,
	}
}

type MetricsSnapshot struct {
	LLMCalls       int
	LLMTokens      int
	LLMAvgLatency  time.Duration
	ToolCalls      int
	ToolErrors     int
	TasksStarted   int
	TasksCompleted int
	TasksFailed    int
}

// Logger creates a structured logger with trace context.
func Logger(base *slog.Logger, traceID TraceID) *slog.Logger {
	return base.With("trace_id", string(traceID))
}

// Context key for trace ID
type ctxKeyTraceID struct{}

func WithTraceID(ctx context.Context, traceID TraceID) context.Context {
	return context.WithValue(ctx, ctxKeyTraceID{}, traceID)
}

func GetTraceID(ctx context.Context) TraceID {
	if v := ctx.Value(ctxKeyTraceID{}); v != nil {
		return v.(TraceID)
	}
	return ""
}
