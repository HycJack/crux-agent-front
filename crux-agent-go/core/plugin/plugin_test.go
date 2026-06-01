package plugin

import (
	"testing"
)

type mockPlugin struct {
	name   string
	inited bool
}

func (m *mockPlugin) Name() string                { return m.name }
func (m *mockPlugin) Init(config map[string]any) error { m.inited = true; return nil }
func (m *mockPlugin) Shutdown() error              { return nil }

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	p := &mockPlugin{name: "test"}
	r.Register(p)

	got, ok := r.Get("test")
	if !ok {
		t.Fatal("expected to find plugin")
	}
	if got.Name() != "test" {
		t.Fatalf("expected test, got %s", got.Name())
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockPlugin{name: "a"})
	r.Register(&mockPlugin{name: "b"})

	names := r.List()
	if len(names) != 2 {
		t.Fatalf("expected 2, got %d", len(names))
	}
}

func TestRegistryInitAll(t *testing.T) {
	r := NewRegistry()
	p := &mockPlugin{name: "test"}
	r.Register(p)

	if err := r.InitAll(nil); err != nil {
		t.Fatal(err)
	}
	if !p.inited {
		t.Fatal("expected plugin to be initialized")
	}
}
