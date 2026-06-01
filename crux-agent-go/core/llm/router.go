package llm

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/hermes-go/core/types"
)

// Router manages multiple LLM providers with fallback.
type Router struct {
	providers map[string]Provider
	order     []string // fallback order
	logger    *slog.Logger
}

func NewRouter() *Router {
	return &Router{
		providers: make(map[string]Provider),
		logger:    slog.Default(),
	}
}

// Register adds a provider with a name.
func (r *Router) Register(name string, provider Provider) {
	r.providers[name] = provider
	r.order = append(r.order, name)
}

// SetFallbackOrder sets the provider fallback order.
func (r *Router) SetFallbackOrder(order []string) {
	r.order = order
}

// Complete tries providers in fallback order.
func (r *Router) Complete(messages []types.Message, tools []types.ToolSchema) (*Response, error) {
	var lastErr error
	for _, name := range r.order {
		provider, ok := r.providers[name]
		if !ok {
			continue
		}
		r.logger.Debug("llm trying provider", "provider", name)
		resp, err := provider.Complete(messages, tools)
		if err == nil {
			return resp, nil
		}
		r.logger.Warn("llm provider failed", "provider", name, "error", err)
		lastErr = err
	}
	return nil, fmt.Errorf("all providers failed, last error: %w", lastErr)
}

// Model returns the first provider's model name.
func (r *Router) Model() string {
	for _, name := range r.order {
		if p, ok := r.providers[name]; ok {
			return p.Model()
		}
	}
	return "unknown"
}

// ProviderNames returns registered provider names.
func (r *Router) ProviderNames() []string {
	return r.order
}

// Retry wraps a provider with retry logic.
type RetryProvider struct {
	inner      Provider
	maxRetries int
	backoff    time.Duration
}

func WithRetry(provider Provider, maxRetries int, backoff time.Duration) *RetryProvider {
	return &RetryProvider{inner: provider, maxRetries: maxRetries, backoff: backoff}
}

func (r *RetryProvider) Complete(messages []types.Message, tools []types.ToolSchema) (*Response, error) {
	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		resp, err := r.inner.Complete(messages, tools)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if attempt < r.maxRetries {
			time.Sleep(r.backoff * time.Duration(1<<uint(attempt)))
		}
	}
	return nil, fmt.Errorf("retry exhausted: %w", lastErr)
}

func (r *RetryProvider) Model() string { return r.inner.Model() }
