// Package commands provides a central command registry.
// Inspired by Hermes Python's hermes_cli/commands.py: COMMAND_REGISTRY.
package commands

import (
	"fmt"
	"sort"
	"strings"
)

// Def defines a command.
type Def struct {
	Name        string   // Canonical name (without slash)
	Description string   // Human-readable description
	Category    string   // "Session", "Config", "Tools", "Info", "Exit"
	Aliases     []string // Alternative names
	ArgsHint    string   // Argument placeholder (e.g. "<prompt>")
	CLIOnly     bool     // Only in interactive CLI
	GatewayOnly bool     // Only in messaging platforms
	Handler     func(args string) error
}

// Registry is the central command registry.
var Registry = []Def{}

// Register adds a command to the registry.
func Register(cmd Def) {
	Registry = append(Registry, cmd)
}

// Resolve finds a command by name or alias.
func Resolve(name string) *Def {
	name = strings.ToLower(strings.TrimPrefix(name, "/"))
	for i := range Registry {
		if Registry[i].Name == name {
			return &Registry[i]
		}
		for _, alias := range Registry[i].Aliases {
			if alias == name {
				return &Registry[i]
			}
		}
	}
	return nil
}

// Execute runs a command by name.
func Execute(name, args string) error {
	cmd := Resolve(name)
	if cmd == nil {
		return fmt.Errorf("unknown command: %s", name)
	}
	if cmd.Handler == nil {
		return fmt.Errorf("no handler for command: %s", name)
	}
	return cmd.Handler(args)
}

// ByCategory returns commands grouped by category.
func ByCategory() map[string][]Def {
	groups := map[string][]Def{}
	for _, cmd := range Registry {
		groups[cmd.Category] = append(groups[cmd.Category], cmd)
	}
	return groups
}

// Names returns all command names.
func Names() []string {
	var names []string
	for _, cmd := range Registry {
		names = append(names, cmd.Name)
	}
	sort.Strings(names)
	return names
}

// Help returns help text for all commands.
func Help() string {
	groups := ByCategory()
	categories := []string{"Session", "Config", "Tools", "Info", "Exit"}
	var sb strings.Builder
	for _, cat := range categories {
		cmds, ok := groups[cat]
		if !ok || len(cmds) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("\n%s:\n", cat))
		for _, cmd := range cmds {
			aliases := ""
			if len(cmd.Aliases) > 0 {
				aliases = fmt.Sprintf(" (%s)", strings.Join(cmd.Aliases, ", "))
			}
			sb.WriteString(fmt.Sprintf("  /%s %s%s — %s\n", cmd.Name, cmd.ArgsHint, aliases, cmd.Description))
		}
	}
	return sb.String()
}

// RegisterBuiltin registers the built-in commands.
func RegisterBuiltin(handlers map[string]func(string) error) {
	h := func(name string) func(string) error {
		if fn, ok := handlers[name]; ok {
			return fn
		}
		return func(args string) error { return fmt.Errorf("not implemented: %s", name) }
	}

	Register(Def{Name: "quit", Description: "Exit the agent", Category: "Exit", Aliases: []string{"exit", "q"}, Handler: h("quit")})
	Register(Def{Name: "new", Description: "Start a new session", Category: "Session", Aliases: []string{"n"}, Handler: h("new")})
	Register(Def{Name: "list", Description: "List saved sessions", Category: "Session", Aliases: []string{"ls"}, Handler: h("list")})
	Register(Def{Name: "load", Description: "Load a session", Category: "Session", ArgsHint: "<id>", Handler: h("load")})
	Register(Def{Name: "memory", Description: "Show memories", Category: "Info", Aliases: []string{"mem"}, Handler: h("memory")})
	Register(Def{Name: "stats", Description: "Session stats", Category: "Info", Handler: h("stats")})
	Register(Def{Name: "help", Description: "Show help", Category: "Info", Aliases: []string{"h", "?"}, Handler: h("help")})
	Register(Def{Name: "tools", Description: "List registered tools", Category: "Tools", Handler: h("tools")})
	Register(Def{Name: "plugins", Description: "List loaded plugins", Category: "Tools", Handler: h("plugins")})
	Register(Def{Name: "skills", Description: "List loaded skills", Category: "Tools", Handler: h("skills")})
}
