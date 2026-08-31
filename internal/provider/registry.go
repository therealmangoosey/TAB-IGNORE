package provider

import (
	"context"
	"fmt"
)

// Registry owns the explicitly enabled providers.
type Registry struct {
	items map[string]Provider
}

func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{items: make(map[string]Provider, len(providers))}
	for _, p := range providers {
		r.items[p.Name()] = p
	}
	return r
}

func (r *Registry) Get(name string) (Provider, error) {
	p, ok := r.items[name]
	if !ok {
		return nil, fmt.Errorf("provider %q is not enabled", name)
	}
	return p, nil
}

func (r *Registry) Resolve(ctx context.Context, name, input string) ([]Source, error) {
	p, err := r.Get(name)
	if err != nil {
		return nil, err
	}
	return p.Resolve(ctx, input)
}
