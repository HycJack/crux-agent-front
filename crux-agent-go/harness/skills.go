package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Skill represents a loaded skill from SKILL.md.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
	FilePath    string `json:"file_path"`
}

// LoadSkills loads skills from directories by finding SKILL.md files.
func LoadSkills(dirs ...string) ([]Skill, []string) {
	var skills []Skill
	var warnings []string

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				warnings = append(warnings, fmt.Sprintf("cannot read %s: %v", dir, err))
			}
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				// Check for SKILL.md in subdirectory
				skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
				if skill, err := loadSkillFile(skillPath); err == nil {
					skills = append(skills, skill)
				}
			} else if entry.Name() == "SKILL.md" {
				skillPath := filepath.Join(dir, entry.Name())
				if skill, err := loadSkillFile(skillPath); err == nil {
					skills = append(skills, skill)
				}
			}
		}
	}

	return skills, warnings
}

func loadSkillFile(path string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}

	content := string(data)
	skill := Skill{
		FilePath: path,
		Content:  content,
	}

	// Parse YAML frontmatter
	if strings.HasPrefix(content, "---") {
		lines := strings.Split(content, "\n")
		inFrontmatter := false
		frontmatterEnd := 0

		for i, line := range lines {
			if i == 0 && strings.TrimSpace(line) == "---" {
				inFrontmatter = true
				continue
			}
			if inFrontmatter && strings.TrimSpace(line) == "---" {
				frontmatterEnd = i
				break
			}
			if inFrontmatter {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					switch key {
					case "name":
						skill.Name = value
					case "description":
						skill.Description = value
					}
				}
			}
		}

		if frontmatterEnd > 0 {
			skill.Content = strings.Join(lines[frontmatterEnd+1:], "\n")
		}
	}

	// Fallback name from filename
	if skill.Name == "" {
		skill.Name = strings.TrimSuffix(filepath.Base(path), ".md")
	}

	return skill, nil
}

// FormatSkillsForSystemPrompt formats skills into a system prompt block.
func FormatSkillsForSystemPrompt(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("The following skills provide specialized instructions for specific tasks.\n")
	sb.WriteString("Read the full skill when the task matches its description.\n\n")
	sb.WriteString("<available_skills>\n")

	for _, skill := range skills {
		sb.WriteString("  <skill>\n")
		sb.WriteString(fmt.Sprintf("    <name>%s</name>\n", escapeXML(skill.Name)))
		sb.WriteString(fmt.Sprintf("    <description>%s</description>\n", escapeXML(skill.Description)))
		sb.WriteString(fmt.Sprintf("    <location>%s</location>\n", escapeXML(skill.FilePath)))
		sb.WriteString("  </skill>\n")
	}

	sb.WriteString("</available_skills>\n")
	return sb.String()
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// LoadPromptTemplates loads prompt template files from a directory.
func LoadPromptTemplates(dir string) map[string]string {
	templates := make(map[string]string)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return templates
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		templates[name] = string(data)
	}
	return templates
}
