package provider

import "context"

// Source represents a candidate media source returned by a provider.
type Source struct {
	URL      string
	Format   string
	Quality  string
	Provider string
}

// Provider is the deliberately small interface implemented by source adapters.
type Provider interface {
	Name() string
	Resolve(ctx context.Context, input string) ([]Source, error)
}
