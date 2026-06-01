package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Registry manages system prompt templates.
type Registry struct {
	dir      string
	templates map[string]string
	mu       sync.RWMutex
}

func NewRegistry(dir string) *Registry {
	return &Registry{
		dir:       dir,
		templates: make(map[string]string),
	}
}

// Load loads all .md templates from the directory.
func (r *Registry) Load() error {
	if r.dir == "" {
		return nil
	}
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(r.dir, e.Name()))
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		r.mu.Lock()
		r.templates[name] = string(data)
		r.mu.Unlock()
	}
	return nil
}

// Get returns a template by name.
func (r *Registry) Get(name string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tmpl, ok := r.templates[name]
	if !ok {
		return "", fmt.Errorf("template %q not found", name)
	}
	return tmpl, nil
}

// Set registers a template.
func (r *Registry) Set(name, content string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.templates[name] = content
}

// Render renders a template with variable substitution.
// Variables: {{var_name}} → value from vars map.
func (r *Registry) Render(name string, vars map[string]string) (string, error) {
	tmpl, err := r.Get(name)
	if err != nil {
		return "", err
	}
	result := tmpl
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result, nil
}

// List returns all template names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var names []string
	for name := range r.templates {
		names = append(names, name)
	}
	return names
}
