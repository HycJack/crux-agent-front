// Package skin provides data-driven CLI theming.
// Inspired by Hermes Python's hermes_cli/skin_engine.py.
package skin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skin defines a CLI theme.
type Skin struct {
	Name    string   `yaml:"name"`
	Banner  Banner   `yaml:"banner"`
	Spinner Spinner  `yaml:"spinner"`
	Tool    Tool     `yaml:"tool"`
	Response Response `yaml:"response"`
	Brand   Brand    `yaml:"brand"`
}

type Banner struct {
	Text  string `yaml:"text"`
	Color string `yaml:"color"`
	Style string `yaml:"style"` // "bold", "dim", "italic"
}

type Spinner struct {
	Faces  []string `yaml:"faces"`
	Verbs  []string `yaml:"verbs"`
	Wings  []string `yaml:"wings"`
	Speed  int      `yaml:"speed"` // ms per frame
}

type Tool struct {
	Prefix string `yaml:"prefix"` // e.g. "🔧" or "┊"
	Color  string `yaml:"color"`
}

type Response struct {
	Border string `yaml:"border"` // "round", "square", "none"
	Color  string `yaml:"color"`
	Padding int   `yaml:"padding"`
}

type Brand struct {
	Text    string `yaml:"text"`
	Version bool   `yaml:"version"`
}

// Default skin.
var Default = Skin{
	Name: "default",
	Banner: Banner{
		Text:  "HERMES",
		Color: "#00ff00",
		Style: "bold",
	},
	Spinner: Spinner{
		Faces: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		Verbs: []string{"thinking", "processing", "computing", "reasoning"},
		Speed: 80,
	},
	Tool: Tool{
		Prefix: "┊",
		Color:  "#888888",
	},
	Response: Response{
		Border:  "round",
		Color:   "#ffffff",
		Padding: 1,
	},
	Brand: Brand{
		Text:    "Hermes Agent",
		Version: true,
	},
}

// Load loads a skin from a YAML file.
func Load(path string) (*Skin, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var skin Skin
	if err := yaml.Unmarshal(data, &skin); err != nil {
		return nil, err
	}
	return &skin, nil
}

// LoadFromDir loads a skin by name from a directory.
func LoadFromDir(dir, name string) (*Skin, error) {
	path := filepath.Join(dir, name+".yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("skin %q not found in %s", name, dir)
	}
	return Load(path)
}

// ListSkins lists available skins in a directory.
func ListSkins(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
		}
	}
	return names
}

// RenderBanner renders the banner with ANSI colors.
func (s *Skin) RenderBanner() string {
	return fmt.Sprintf("\033[1;32m%s\033[0m", s.Banner.Text)
}

// NextSpinner returns the next spinner frame.
func (s *Skin) NextSpinner(frame int) string {
	if len(s.Spinner.Faces) == 0 {
		return "⠋"
	}
	return s.Spinner.Faces[frame%len(s.Spinner.Faces)]
}

// RandomVerb returns a random verb.
func (s *Skin) RandomVerb(idx int) string {
	if len(s.Spinner.Verbs) == 0 {
		return "thinking"
	}
	return s.Spinner.Verbs[idx%len(s.Spinner.Verbs)]
}

// FormatTool formats a tool call message.
func (s *Skin) FormatTool(name, args string) string {
	return fmt.Sprintf("%s %s(%s)", s.Tool.Prefix, name, truncate(args, 80))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
