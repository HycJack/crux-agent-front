package bus

import (
	"sync/atomic"
	"testing"
)

func TestBusSubscribeEmit(t *testing.T) {
	b := New()
	count := 0
	b.Subscribe("test", "h1", func(e Event) error {
		count++
		return nil
	})

	b.Emit(Event{Type: "test", Source: "src", Payload: "hello"})
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}

	b.Emit(Event{Type: "test", Source: "src", Payload: "world"})
	if count != 2 {
		t.Fatalf("expected 2, got %d", count)
	}
}

func TestBusMultipleHandlers(t *testing.T) {
	b := New()
	var count atomic.Int32
	b.Subscribe("evt", "a", func(e Event) error { count.Add(1); return nil })
	b.Subscribe("evt", "b", func(e Event) error { count.Add(1); return nil })

	b.Emit(Event{Type: "evt", Source: "src"})
	if count.Load() != 2 {
		t.Fatalf("expected 2, got %d", count.Load())
	}
}

func TestBusUnsubscribe(t *testing.T) {
	b := New()
	count := 0
	b.Subscribe("evt", "h1", func(e Event) error { count++; return nil })
	b.Unsubscribe("evt", "h1")

	b.Emit(Event{Type: "evt", Source: "src"})
	if count != 0 {
		t.Fatalf("expected 0 after unsubscribe, got %d", count)
	}
}

func TestBusWildcard(t *testing.T) {
	b := New()
	count := 0
	b.Subscribe("*", "h1", func(e Event) error {
		count++
		return nil
	})

	b.Emit(Event{Type: "any", Source: "src"})
	// Wildcard is not supported in current impl, so count should be 0
	if count != 0 {
		t.Fatalf("wildcard not supported, expected 0, got %d", count)
	}
}

func TestBusNoMatch(t *testing.T) {
	b := New()
	count := 0
	b.Subscribe("foo", "h1", func(e Event) error { count++; return nil })

	b.Emit(Event{Type: "bar", Source: "src"})
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
}

func TestBusCount(t *testing.T) {
	b := New()
	b.Subscribe("evt", "a", func(e Event) error { return nil })
	b.Subscribe("evt", "b", func(e Event) error { return nil })

	if c := b.Count("evt"); c != 2 {
		t.Fatalf("expected 2, got %d", c)
	}
	if c := b.Count("other"); c != 0 {
		t.Fatalf("expected 0, got %d", c)
	}
}

func TestBusString(t *testing.T) {
	b := New()
	b.Subscribe("evt", "a", func(e Event) error { return nil })
	s := b.String()
	if s == "" {
		t.Fatal("expected non-empty string")
	}
}
