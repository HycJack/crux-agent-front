package main

import (
	"fmt"
	"log"
)

// ════════════════════════════════════════════════
// Skill Management Tools — skill_manage, skill_list
// ════════════════════════════════════════════════

type SkillManageTool struct {
	engine *ChatEngine
}

func NewSkillManageTool() *SkillManageTool {
	return &SkillManageTool{}
}

func (s *SkillManageTool) SetEngine(e *ChatEngine) {
	s.engine = e
}

func (s *SkillManageTool) ToolSchemas() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "skill_manage",
				"description": "Create, update, or manage runtime skills. Skills are reusable procedures extracted from successful tool call patterns. Use 'create' to save a new skill, 'update' to improve an existing one, 'archive' to mark as stale.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"action": map[string]interface{}{
							"type":        "string",
							"description": "Action: create, update, archive, pin, unpin",
							"enum":        []string{"create", "update", "archive", "pin", "unpin"},
						},
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Skill name (for create/update)",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "What this skill does (for create/update)",
						},
						"trigger": map[string]interface{}{
							"type":        "string",
							"description": "When to use this skill (for create)",
						},
						"tools": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "Tool names this skill uses",
						},
						"prompt": map[string]interface{}{
							"type":        "string",
							"description": "System prompt addition for this skill",
						},
						"skill_id": map[string]interface{}{
							"type":        "string",
							"description": "Skill ID (for update/archive/pin/unpin)",
						},
					},
					"required": []string{"action"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "skill_list",
				"description": "List available skills. Shows both builtin and runtime-created skills with their usage stats.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filter": map[string]interface{}{
							"type":        "string",
							"description": "Filter: all, builtin, runtime, active, archived, pinned",
						},
					},
				},
			},
		},
	}
}

func (s *SkillManageTool) ToolNames() []string {
	return []string{"skill_manage", "skill_list"}
}

func (s *SkillManageTool) Handle(name string, args map[string]interface{}) (string, error) {
	switch name {
	case "skill_manage":
		return s.manageSkill(args)
	case "skill_list":
		return s.listSkills(args)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func (s *SkillManageTool) manageSkill(args map[string]interface{}) (string, error) {
	action, _ := args["action"].(string)
	if action == "" {
		return "", fmt.Errorf("action is required")
	}

	switch action {
	case "create":
		return s.createSkill(args)
	case "update":
		return s.updateSkill(args)
	case "archive":
		return s.archiveSkill(args)
	case "pin":
		return s.pinSkill(args, true)
	case "unpin":
		return s.pinSkill(args, false)
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (s *SkillManageTool) createSkill(args map[string]interface{}) (string, error) {
	name, _ := args["name"].(string)
	desc, _ := args["description"].(string)
	trigger, _ := args["trigger"].(string)
	prompt, _ := args["prompt"].(string)

	if name == "" {
		return "", fmt.Errorf("name is required for create")
	}

	// Extract tools array
	var tools []string
	if toolsRaw, ok := args["tools"].([]interface{}); ok {
		for _, t := range toolsRaw {
			if ts, ok := t.(string); ok {
				tools = append(tools, ts)
			}
		}
	}

	skill := &UserSkill{
		ID:          fmt.Sprintf("skill-%d", len(s.engine.skillStore.skills)+1),
		Name:        name,
		Description: desc,
		Trigger:     trigger,
		Tools:       tools,
		Prompt:      prompt,
		Builtin:     false,
		CreatedBy:   "agent",
		Archived:    false,
		Pinned:      false,
	}

	if err := s.engine.skillStore.Create(skill); err != nil {
		return "", fmt.Errorf("failed to create skill: %w", err)
	}

	log.Printf("[SKILL] Created skill: %s (%s)", skill.ID, name)
	return fmt.Sprintf("Created skill: %s (id: %s). Tools: %v", name, skill.ID, tools), nil
}

func (s *SkillManageTool) updateSkill(args map[string]interface{}) (string, error) {
	skillID, _ := args["skill_id"].(string)
	if skillID == "" {
		return "", fmt.Errorf("skill_id is required for update")
	}

	skill := s.engine.skillStore.Get(skillID)
	if skill == nil {
		return "", fmt.Errorf("skill %s not found", skillID)
	}

	if name, ok := args["name"].(string); ok && name != "" {
		skill.Name = name
	}
	if desc, ok := args["description"].(string); ok && desc != "" {
		skill.Description = desc
	}
	if prompt, ok := args["prompt"].(string); ok && prompt != "" {
		skill.Prompt = prompt
	}
	if trigger, ok := args["trigger"].(string); ok && trigger != "" {
		skill.Trigger = trigger
	}

	log.Printf("[SKILL] Updated skill: %s", skillID)
	return fmt.Sprintf("Updated skill: %s (%s)", skill.Name, skillID), nil
}

func (s *SkillManageTool) archiveSkill(args map[string]interface{}) (string, error) {
	skillID, _ := args["skill_id"].(string)
	if skillID == "" {
		return "", fmt.Errorf("skill_id is required")
	}

	skill := s.engine.skillStore.Get(skillID)
	if skill == nil {
		return "", fmt.Errorf("skill %s not found", skillID)
	}

	if skill.Builtin {
		return "", fmt.Errorf("cannot archive builtin skill")
	}

	if err := s.engine.skillStore.Archive(skillID, true); err != nil {
		return "", fmt.Errorf("failed to archive skill: %w", err)
	}

	log.Printf("[SKILL] Archived skill: %s (%s)", skillID, skill.Name)
	return fmt.Sprintf("Archived skill: %s (%s)", skill.Name, skillID), nil
}

func (s *SkillManageTool) pinSkill(args map[string]interface{}, pin bool) (string, error) {
	skillID, _ := args["skill_id"].(string)
	if skillID == "" {
		return "", fmt.Errorf("skill_id is required")
	}

	skill := s.engine.skillStore.Get(skillID)
	if skill == nil {
		return "", fmt.Errorf("skill %s not found", skillID)
	}

	if err := s.engine.skillStore.Pin(skillID, pin); err != nil {
		return "", fmt.Errorf("failed to pin/unpin skill: %w", err)
	}

	action := "Pinned"
	if !pin {
		action = "Unpinned"
	}
	log.Printf("[SKILL] %s skill: %s (%s)", action, skillID, skill.Name)
	return fmt.Sprintf("%s skill: %s (%s)", action, skill.Name, skillID), nil
}

func (s *SkillManageTool) listSkills(args map[string]interface{}) (string, error) {
	filter := "all"
	if f, ok := args["filter"].(string); ok && f != "" {
		filter = f
	}

	// Collect all skills (builtin + from all users)
	var allSkills []*UserSkill
	seen := make(map[string]bool)
	for _, sk := range s.engine.skillStore.ListBuiltin() {
		if !seen[sk.ID] {
			allSkills = append(allSkills, sk)
			seen[sk.ID] = true
		}
	}
	// Also get non-builtin skills from the store directly
	s.engine.skillStore.mu.RLock()
	for _, sk := range s.engine.skillStore.skills {
		if !seen[sk.ID] {
			allSkills = append(allSkills, sk)
			seen[sk.ID] = true
		}
	}
	s.engine.skillStore.mu.RUnlock()

	var result string
	count := 0
	for _, sk := range allSkills {
		switch filter {
		case "builtin":
			if !sk.Builtin {
				continue
			}
		case "runtime":
			if sk.Builtin {
				continue
			}
		case "active":
			if sk.Archived {
				continue
			}
		case "archived":
			if !sk.Archived {
				continue
			}
		case "pinned":
			if !sk.Pinned {
				continue
			}
		}

		pinned := ""
		if sk.Pinned {
			pinned = " 📌"
		}
		state := "active"
		if sk.Archived {
			state = "archived"
		}

		result += fmt.Sprintf("- [%s] %s%s\n  %s\n  State: %s | Tools: %v | Created by: %s\n\n",
			sk.ID, sk.Name, pinned, sk.Description, state, sk.Tools, sk.CreatedBy)
		count++
	}

	if count == 0 {
		return fmt.Sprintf("No skills found with filter: %s", filter), nil
	}

	return fmt.Sprintf("Skills (%d):\n\n%s", count, result), nil
}
