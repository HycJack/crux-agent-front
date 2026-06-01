// Package tools provides a global tool registry with auto-discovery.
// Inspired by Hermes Python's tools/registry.py: register() at import time.
package tools

import (
	"fmt"
	"sort"
	"sync"

	"github.com/hermes-go/core/agent"
)

// Entry is a registered tool.
type Entry struct {
	Name     string
	Schema   agent.AgentTool
	Source   string // "builtin", "skill", "plugin"
	Category string // "file", "system", "terminal", "memory", etc.
}

var (
	mu       sync.RWMutex
	registry = map[string]*Entry{}
)

// Register adds a tool to the global registry.
func Register(name string, tool agent.AgentTool, source, category string) {
	mu.Lock()
	defer mu.Unlock()
	registry[name] = &Entry{
		Name:     name,
		Schema:   tool,
		Source:   source,
		Category: category,
	}
}

// Get returns a tool by name.
func Get(name string) (*Entry, bool) {
	mu.RLock()
	defer mu.RUnlock()
	e, ok := registry[name]
	return e, ok
}

// List returns all registered tools.
func List() []Entry {
	mu.RLock()
	defer mu.RUnlock()
	var entries []Entry
	for _, e := range registry {
		entries = append(entries, *e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries
}

// ByCategory returns tools filtered by category.
func ByCategory(category string) []Entry {
	mu.RLock()
	defer mu.RUnlock()
	var entries []Entry
	for _, e := range registry {
		if e.Category == category {
			entries = append(entries, *e)
		}
	}
	return entries
}

// BySource returns tools filtered by source.
func BySource(source string) []Entry {
	mu.RLock()
	defer mu.RUnlock()
	var entries []Entry
	for _, e := range registry {
		if e.Source == source {
			entries = append(entries, *e)
		}
	}
	return entries
}

// AgentTools returns all registered tools as AgentTool slice.
func AgentTools() []agent.AgentTool {
	mu.RLock()
	defer mu.RUnlock()
	var tools []agent.AgentTool
	for _, e := range registry {
		tools = append(tools, e.Schema)
	}
	return tools
}

// Clear removes all registered tools.
func Clear() {
	mu.Lock()
	defer mu.Unlock()
	registry = map[string]*Entry{}
}

// String returns a summary of registered tools.
func String() string {
	mu.RLock()
	defer mu.RUnlock()
	categories := map[string]int{}
	sources := map[string]int{}
	for _, e := range registry {
		categories[e.Category]++
		sources[e.Source]++
	}
	return fmt.Sprintf("Tools: %d total, categories=%v, sources=%v", len(registry), categories, sources)
}
