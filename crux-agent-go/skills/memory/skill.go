package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hermes-go/core/types"
)

// Skill provides persistent memory via MEMORY.md and USER.md files.
type Skill struct {
	dir      string
	memories []Memory
}

type Memory struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Category  string    `json:"category"`
	UpdatedAt time.Time `json:"updated_at"`
}

func New(dir string) *Skill {
	return &Skill{dir: dir}
}

func (s *Skill) Name() string { return "memory" }

func (s *Skill) Init(ctx interface{}) error {
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return err
	}
	return s.load()
}

func (s *Skill) Shutdown() error {
	return s.save()
}

func (s *Skill) Capabilities() interface{} {
	return nil
}

func (s *Skill) ToolSchemas() []types.ToolSchema {
	return []types.ToolSchema{
		{
			Name:        "memory_save",
			Description: "Save a memory (key-value pair with category).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key":      map[string]any{"type": "string", "description": "Memory key/identifier"},
					"value":    map[string]any{"type": "string", "description": "Memory content"},
					"category": map[string]any{"type": "string", "description": "Category (user/memory/env/skill)", "default": "memory"},
				},
				"required": []string{"key", "value"},
			},
		},
		{
			Name:        "memory_search",
			Description: "Search memories by keyword.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":    map[string]any{"type": "string", "description": "Search query"},
					"category": map[string]any{"type": "string", "description": "Filter by category"},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "memory_list",
			Description: "List all memories, optionally filtered by category.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"category": map[string]any{"type": "string", "description": "Filter by category"},
				},
			},
		},
		{
			Name:        "memory_delete",
			Description: "Delete a memory by key.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key": map[string]any{"type": "string", "description": "Memory key to delete"},
				},
				"required": []string{"key"},
			},
		},
	}
}

func (s *Skill) Handle(call types.ToolCall) types.ToolResult {
	var args map[string]any
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return types.ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Error: %v", err), IsError: true}
	}

	var content string
	var err error

	switch call.Function.Name {
	case "memory_save":
		err = s.saveMemory(args)
		if err == nil {
			content = "Memory saved."
		}
	case "memory_search":
		content, err = s.searchMemory(args)
	case "memory_list":
		content, err = s.listMemory(args)
	case "memory_delete":
		err = s.deleteMemory(args)
		if err == nil {
			content = "Memory deleted."
		}
	default:
		err = fmt.Errorf("unknown tool: %s", call.Function.Name)
	}

	result := types.ToolResult{ToolCallID: call.ID}
	if err != nil {
		result.Content = fmt.Sprintf("Error: %v", err)
		result.IsError = true
	} else {
		result.Content = content
	}
	return result
}

// GetMemoryText returns all memories as formatted text for context injection.
func (s *Skill) GetMemoryText() string {
	if len(s.memories) == 0 {
		return ""
	}
	var sb strings.Builder
	// Sort by category then key
	sort.Slice(s.memories, func(i, j int) bool {
		if s.memories[i].Category != s.memories[j].Category {
			return s.memories[i].Category < s.memories[j].Category
		}
		return s.memories[i].Key < s.memories[j].Key
	})
	currentCat := ""
	for _, m := range s.memories {
		if m.Category != currentCat {
			currentCat = m.Category
			sb.WriteString(fmt.Sprintf("\n[%s]\n", currentCat))
		}
		sb.WriteString(fmt.Sprintf("- %s: %s\n", m.Key, m.Value))
	}
	return sb.String()
}

func (s *Skill) saveMemory(args map[string]any) error {
	key, _ := args["key"].(string)
	value, _ := args["value"].(string)
	category, _ := args["category"].(string)
	if category == "" {
		category = "memory"
	}
	if key == "" || value == "" {
		return fmt.Errorf("key and value are required")
	}

	// Update existing or add new
	found := false
	for i, m := range s.memories {
		if m.Key == key {
			s.memories[i].Value = value
			s.memories[i].Category = category
			s.memories[i].UpdatedAt = time.Now()
			found = true
			break
		}
	}
	if !found {
		s.memories = append(s.memories, Memory{
			Key:       key,
			Value:     value,
			Category:  category,
			UpdatedAt: time.Now(),
		})
	}
	return s.save()
}

func (s *Skill) searchMemory(args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	category, _ := args["category"].(string)

	var results []Memory
	for _, m := range s.memories {
		if category != "" && m.Category != category {
			continue
		}
		if strings.Contains(strings.ToLower(m.Key), strings.ToLower(query)) ||
			strings.Contains(strings.ToLower(m.Value), strings.ToLower(query)) {
			results = append(results, m)
		}
	}

	if len(results) == 0 {
		return "No matching memories found.", nil
	}

	var sb strings.Builder
	for _, m := range results {
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", m.Category, m.Key, m.Value))
	}
	return sb.String(), nil
}

func (s *Skill) listMemory(args map[string]any) (string, error) {
	category, _ := args["category"].(string)

	var filtered []Memory
	for _, m := range s.memories {
		if category == "" || m.Category == category {
			filtered = append(filtered, m)
		}
	}

	if len(filtered) == 0 {
		return "No memories.", nil
	}

	var sb strings.Builder
	for _, m := range filtered {
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", m.Category, m.Key, m.Value))
	}
	return sb.String(), nil
}

func (s *Skill) deleteMemory(args map[string]any) error {
	key, _ := args["key"].(string)
	if key == "" {
		return fmt.Errorf("key is required")
	}

	for i, m := range s.memories {
		if m.Key == key {
			s.memories = append(s.memories[:i], s.memories[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("memory %q not found", key)
}

func (s *Skill) load() error {
	path := filepath.Join(s.dir, "memories.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &s.memories)
}

func (s *Skill) save() error {
	path := filepath.Join(s.dir, "memories.json")
	data, err := json.MarshalIndent(s.memories, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
