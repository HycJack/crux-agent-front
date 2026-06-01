package commands

import (
	"testing"
)

func TestResolve(t *testing.T) {
	Registry = nil
	Register(Def{Name: "quit", Aliases: []string{"exit", "q"}, Category: "Exit"})
	Register(Def{Name: "help", Aliases: []string{"h", "?"}, Category: "Info"})

	if cmd := Resolve("quit"); cmd == nil {
		t.Fatal("expected to find quit")
	}
	if cmd := Resolve("exit"); cmd == nil {
		t.Fatal("expected to find exit via alias")
	}
	if cmd := Resolve("q"); cmd == nil {
		t.Fatal("expected to find q via alias")
	}
	if cmd := Resolve("nonexistent"); cmd != nil {
		t.Fatal("expected nil for nonexistent")
	}
}

func TestByCategory(t *testing.T) {
	Registry = nil
	Register(Def{Name: "quit", Category: "Exit"})
	Register(Def{Name: "help", Category: "Info"})
	Register(Def{Name: "stats", Category: "Info"})

	groups := ByCategory()
	if len(groups["Info"]) != 2 {
		t.Fatalf("expected 2 info commands, got %d", len(groups["Info"]))
	}
}

func TestHelp(t *testing.T) {
	Registry = nil
	Register(Def{Name: "quit", Description: "Exit", Category: "Exit"})
	Register(Def{Name: "help", Description: "Show help", Category: "Info"})

	help := Help()
	if help == "" {
		t.Fatal("expected non-empty help")
	}
}
