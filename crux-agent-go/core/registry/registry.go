package registry

import (
	"fmt"
	"sync"

	"github.com/hermes-go/core/transport"
)

// Registry manages known agents and their transports.
type Registry struct {
	mu      sync.RWMutex
	agents  map[string]*RegisteredAgent
}

type RegisteredAgent struct {
	Card      *transport.AgentCard
	Transport transport.Transport
	Status    string // "online", "offline", "busy"
	Location  string // "memory:", "unix:", "https:"
}

func New() *Registry {
	return &Registry{
		agents: make(map[string]*RegisteredAgent),
	}
}

// Register adds an agent to the registry.
func (r *Registry) Register(card *transport.AgentCard, t transport.Transport, location string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[card.Name] = &RegisteredAgent{
		Card:      card,
		Transport: t,
		Status:    "online",
		Location:  location,
	}
}

// Unregister removes an agent.
func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, id)
}

// Get returns a registered agent.
func (r *Registry) Get(id string) (*RegisteredAgent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	agent, ok := r.agents[id]
	if !ok {
		return nil, fmt.Errorf("agent %q not registered", id)
	}
	return agent, nil
}

// List returns all registered agents.
func (r *Registry) List() []*RegisteredAgent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var agents []*RegisteredAgent
	for _, a := range r.agents {
		agents = append(agents, a)
	}
	return agents
}

// Search finds agents by capability or name.
func (r *Registry) Search(query string) []*RegisteredAgent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var results []*RegisteredAgent
	for _, a := range r.agents {
		if contains(a.Card.Name, query) || contains(a.Card.Description, query) {
			results = append(results, a)
			continue
		}
		for _, skill := range a.Card.Skills {
			if contains(skill, query) {
				results = append(results, a)
				break
			}
		}
	}
	return results
}

func contains(s, sub string) bool {
	if len(s) < len(sub) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
