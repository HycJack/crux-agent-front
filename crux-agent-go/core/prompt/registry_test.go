package prompt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistrySetGet(t *testing.T) {
	r := NewRegistry("")
	r.Set("test", "You are {{name}}.")

	tmpl, err := r.Get("test")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if tmpl != "You are {{name}}." {
		t.Errorf("got %s", tmpl)
	}
}

func TestRegistryRender(t *testing.T) {
	r := NewRegistry("")
	r.Set("greeting", "Hello {{user}}, welcome to {{project}}!")

	result, err := r.Render("greeting", map[string]string{
		"user":    "HycJack",
		"project": "Hermes",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if result != "Hello HycJack, welcome to Hermes!" {
		t.Errorf("got %s", result)
	}
}

func TestRegistryLoad(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "coder.md"), []byte("You are a coder."), 0644)
	os.WriteFile(filepath.Join(dir, "helper.md"), []byte("You are a helper."), 0644)

	r := NewRegistry(dir)
	if err := r.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	names := r.List()
	if len(names) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(names))
	}
}

func TestRegistryNotFound(t *testing.T) {
	r := NewRegistry("")
	_, err := r.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}
