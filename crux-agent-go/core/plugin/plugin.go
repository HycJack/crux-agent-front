// Package plugin provides a plugin system for extending hermes-go.
// Inspired by Hermes Python's plugins/: memory, model-providers, context_engine.
package plugin

import (
	"fmt"
	"sync"
)

// Plugin is the base interface all plugins must implement.
type Plugin interface {
	Name() string
	Init(config map[string]any) error
	Shutdown() error
}

// MemoryPlugin extends the memory system.
type MemoryPlugin interface {
	Plugin
	Save(category, key, value string) error
	Search(query string, category string) ([]MemoryEntry, error)
	List(category string) ([]MemoryEntry, error)
	Delete(key string) error
	GetText() string // Formatted text for context injection
}

// ModelPlugin provides an alternative LLM backend.
type ModelPlugin interface {
	Plugin
	Complete(messages []Message, tools []Tool) (*Response, error)
	Stream(messages []Message, tools []Tool) (<-chan StreamEvent, error)
}

// ContextPlugin modifies the context before sending to LLM.
type ContextPlugin interface {
	Plugin
	Transform(messages []Message) []Message
}

// MemoryEntry is a stored memory.
type MemoryEntry struct {
	Category  string `json:"category"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	Timestamp int64  `json:"timestamp"`
}

// Message is a simplified message for plugin use.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Tool is a simplified tool definition for plugin use.
type Tool struct {
	Name       string         `json:"name"`
	Parameters map[string]any `json:"parameters"`
}

// Response is a simplified LLM response.
type Response struct {
	Content string `json:"content"`
	Error   error  `json:"error,omitempty"`
}

// StreamEvent is a streaming event.
type StreamEvent struct {
	Type    string `json:"type"` // "text_delta", "tool_call", "done", "error"
	Content string `json:"content,omitempty"`
	Error   error  `json:"error,omitempty"`
}

// Registry manages loaded plugins.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
}

// NewRegistry creates a new plugin registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]Plugin),
	}
}

// Register adds a plugin.
func (r *Registry) Register(p Plugin) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins[p.Name()] = p
}

// Get returns a plugin by name.
func (r *Registry) Get(name string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[name]
	return p, ok
}

// GetMemory returns the first MemoryPlugin, if any.
func (r *Registry) GetMemory() MemoryPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		if mp, ok := p.(MemoryPlugin); ok {
			return mp
		}
	}
	return nil
}

// GetModel returns the first ModelPlugin, if any.
func (r *Registry) GetModel() ModelPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		if mp, ok := p.(ModelPlugin); ok {
			return mp
		}
	}
	return nil
}

// GetContext returns all ContextPlugins.
func (r *Registry) GetContext() []ContextPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var plugins []ContextPlugin
	for _, p := range r.plugins {
		if cp, ok := p.(ContextPlugin); ok {
			plugins = append(plugins, cp)
		}
	}
	return plugins
}

// InitAll initializes all plugins.
func (r *Registry) InitAll(configs map[string]map[string]any) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for name, p := range r.plugins {
		cfg := configs[name]
		if cfg == nil {
			cfg = map[string]any{}
		}
		if err := p.Init(cfg); err != nil {
			return fmt.Errorf("plugin %s init failed: %w", name, err)
		}
	}
	return nil
}

// ShutdownAll shuts down all plugins.
func (r *Registry) ShutdownAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		p.Shutdown()
	}
}

// List returns all plugin names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var names []string
	for name := range r.plugins {
		names = append(names, name)
	}
	return names
}
