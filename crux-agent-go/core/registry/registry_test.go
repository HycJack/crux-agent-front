package registry

import (
	"testing"

	"github.com/hermes-go/core/transport"
)

func TestRegistry(t *testing.T) {
	r := New()

	card := &transport.AgentCard{
		Name:        "coder",
		Description: "Coding agent",
		Skills:      []string{"terminal", "file"},
	}
	r.Register(card, nil, "memory:")

	agent, err := r.Get("coder")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if agent.Card.Name != "coder" {
		t.Errorf("name: %s", agent.Card.Name)
	}
	if agent.Status != "online" {
		t.Errorf("status: %s", agent.Status)
	}
}

func TestRegistryList(t *testing.T) {
	r := New()
	r.Register(&transport.AgentCard{Name: "a"}, nil, "memory:")
	r.Register(&transport.AgentCard{Name: "b"}, nil, "memory:")

	agents := r.List()
	if len(agents) != 2 {
		t.Fatalf("expected 2, got %d", len(agents))
	}
}

func TestRegistrySearch(t *testing.T) {
	r := New()
	r.Register(&transport.AgentCard{Name: "coder", Description: "writes code"}, nil, "memory:")
	r.Register(&transport.AgentCard{Name: "writer", Description: "writes docs"}, nil, "memory:")

	results := r.Search("code")
	if len(results) != 1 || results[0].Card.Name != "coder" {
		t.Errorf("expected coder, got %v", results)
	}
}

func TestRegistryUnregister(t *testing.T) {
	r := New()
	r.Register(&transport.AgentCard{Name: "temp"}, nil, "memory:")
	r.Unregister("temp")

	_, err := r.Get("temp")
	if err == nil {
		t.Fatal("expected error after unregister")
	}
}
