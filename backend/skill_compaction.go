package main

import (
	"fmt"
	"log"
)

// ════════════════════════════════════════════════
// Context Compaction Skill — compact_messages
// ════════════════════════════════════════════════

type CompactionSkill struct {
	engine *ChatEngine
}

func NewCompactionSkill() *CompactionSkill {
	return &CompactionSkill{}
}

func (c *CompactionSkill) SetEngine(e *ChatEngine) {
	c.engine = e
}

func (c *CompactionSkill) ToolSchemas() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "compact_messages",
				"description": "Compress the conversation context by summarizing older messages. Use when the conversation is getting very long and you're losing track of earlier context. Keeps the most recent messages intact.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"session_id": map[string]interface{}{
							"type":        "string",
							"description": "Session ID to compact (optional, defaults to current session)",
						},
						"keep_recent": map[string]interface{}{
							"type":        "integer",
							"description": "Number of recent messages to keep intact (default 10)",
						},
					},
				},
			},
		},
	}
}

func (c *CompactionSkill) ToolNames() []string {
	return []string{"compact_messages"}
}

func (c *CompactionSkill) Handle(name string, args map[string]interface{}) (string, error) {
	switch name {
	case "compact_messages":
		return c.compact(args)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func (c *CompactionSkill) compact(args map[string]interface{}) (string, error) {
	keepRecent := 10
	if v, ok := args["keep_recent"].(float64); ok && v > 0 {
		keepRecent = int(v)
	}

	if c.engine == nil {
		return "", fmt.Errorf("engine not initialized")
	}

	log.Printf("[COMPACTION] Triggered with keep_recent=%d", keepRecent)
	return fmt.Sprintf("Context compaction requested. Keeping last %d messages. The agent loop will handle summarization on the next turn.", keepRecent), nil
}
