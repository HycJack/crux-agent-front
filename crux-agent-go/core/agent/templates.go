// Package agent/templates provides prompt template support.
// Inspired by pi's prompt-templates.ts: reusable templates with argument placeholders.
package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PromptTemplate is a reusable prompt template with argument placeholders.
type PromptTemplate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

// TemplateRegistry manages prompt templates.
type TemplateRegistry struct {
	templates map[string]*PromptTemplate
}

// NewTemplateRegistry creates a new template registry.
func NewTemplateRegistry() *TemplateRegistry {
	return &TemplateRegistry{
		templates: make(map[string]*PromptTemplate),
	}
}

// Register adds a template.
func (r *TemplateRegistry) Register(tmpl *PromptTemplate) {
	r.templates[tmpl.Name] = tmpl
}

// Get returns a template by name.
func (r *TemplateRegistry) Get(name string) (*PromptTemplate, bool) {
	t, ok := r.templates[name]
	return t, ok
}

// List returns all template names.
func (r *TemplateRegistry) List() []string {
	var names []string
	for name := range r.templates {
		names = append(names, name)
	}
	return names
}

// FormatInvocation formats a template invocation with arguments.
// Placeholders are {{arg_name}} format.
func FormatInvocation(template *PromptTemplate, args map[string]string) string {
	result := template.Content
	for key, value := range args {
		placeholder := fmt.Sprintf("{{%s}}", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// LoadTemplates loads prompt template files from a directory.
// Each .md file becomes a template. YAML frontmatter provides metadata.
func LoadTemplates(dir string) ([]*PromptTemplate, error) {
	var templates []*PromptTemplate

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
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

		content := string(data)
		tmpl := &PromptTemplate{
			Name:    strings.TrimSuffix(entry.Name(), ".md"),
			Content: content,
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
							tmpl.Name = value
						case "description":
							tmpl.Description = value
						}
					}
				}
			}

			if frontmatterEnd > 0 {
				tmpl.Content = strings.Join(lines[frontmatterEnd+1:], "\n")
			}
		}

		templates = append(templates, tmpl)
	}

	return templates, nil
}
