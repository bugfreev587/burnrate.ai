package provider

import (
	"fmt"
	"sync"
)

// Registry maintains a map of provider name → adapter instance.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]ProviderAdapter
}

func NewRegistry() *Registry {
	r := &Registry{
		adapters: make(map[string]ProviderAdapter),
	}
	// Register built-in adapters
	r.Register(NewOpenAIAdapter())
	r.Register(NewAnthropicAdapter())
	r.Register(NewDeepSeekAdapter())
	r.Register(NewMistralAdapter())
	return r
}

func (r *Registry) Register(adapter ProviderAdapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[adapter.Name()] = adapter
}

func (r *Registry) Get(provider string) (ProviderAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter, ok := r.adapters[provider]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
	return adapter, nil
}
