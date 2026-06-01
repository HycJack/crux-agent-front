package observe

import "testing"

func TestTracer(t *testing.T) {
	tr := NewTracer()
	traceID := NewTraceID()

	span := tr.Start(traceID, "test-op", map[string]any{"key": "value"})
	tr.End(span)

	spans := tr.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "test-op" {
		t.Errorf("name: %s", spans[0].Name)
	}
	if spans[0].Duration == 0 {
		t.Error("duration should be > 0")
	}
}

func TestMetrics(t *testing.T) {
	m := NewMetrics()
	m.RecordLLM(100, 500_000_000) // 500ms
	m.RecordLLM(200, 300_000_000) // 300ms
	m.RecordTool(false)
	m.RecordTool(true)

	snap := m.Snapshot()
	if snap.LLMCalls != 2 {
		t.Errorf("llm calls: %d", snap.LLMCalls)
	}
	if snap.LLMTokens != 300 {
		t.Errorf("tokens: %d", snap.LLMTokens)
	}
	if snap.ToolCalls != 2 {
		t.Errorf("tool calls: %d", snap.ToolCalls)
	}
	if snap.ToolErrors != 1 {
		t.Errorf("tool errors: %d", snap.ToolErrors)
	}
}
