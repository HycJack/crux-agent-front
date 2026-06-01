package bus

import (
	"fmt"
	"log/slog"
	"sync"
)

// Event is a message on the event bus.
type Event struct {
	Type    string
	Source  string
	Payload any
}

// Handler processes an event.
type Handler func(Event) error

// Bus is a synchronous in-process event bus.
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]handlerEntry
	logger   *slog.Logger
}

type handlerEntry struct {
	id      string
	handler Handler
}

func New() *Bus {
	return &Bus{
		handlers: make(map[string][]handlerEntry),
		logger:   slog.Default(),
	}
}

// Subscribe registers a handler for an event type.
func (b *Bus) Subscribe(eventType, id string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handlerEntry{id: id, handler: handler})
}

// Unsubscribe removes a handler.
func (b *Bus) Unsubscribe(eventType, id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entries := b.handlers[eventType]
	for i, e := range entries {
		if e.id == id {
			b.handlers[eventType] = append(entries[:i], entries[i+1:]...)
			return
		}
	}
}

// Emit sends an event to all subscribed handlers (synchronous).
func (b *Bus) Emit(event Event) {
	b.mu.RLock()
	entries := b.handlers[event.Type]
	b.mu.RUnlock()

	for _, e := range entries {
		if err := e.handler(event); err != nil {
			b.logger.Warn("event handler error", "type", event.Type, "handler", e.id, "error", err)
		}
	}
}

// EmitAsync sends an event to all handlers asynchronously.
func (b *Bus) EmitAsync(event Event) {
	b.mu.RLock()
	entries := b.handlers[event.Type]
	b.mu.RUnlock()

	for _, e := range entries {
		go func(h handlerEntry) {
			if err := h.handler(event); err != nil {
				b.logger.Warn("async event handler error", "type", event.Type, "handler", h.id, "error", err)
			}
		}(e)
	}
}

// Count returns the number of handlers for an event type.
func (b *Bus) Count(eventType string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.handlers[eventType])
}

// String returns a summary of registered handlers.
func (b *Bus) String() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	s := "EventBus{"
	for t, entries := range b.handlers {
		s += fmt.Sprintf(" %s:%d", t, len(entries))
	}
	s += " }"
	return s
}
