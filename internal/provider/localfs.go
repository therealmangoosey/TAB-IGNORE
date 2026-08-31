package provider

import "context"

type LocalFS struct{}

func (LocalFS) Name() string { return "localfs" }

// Resolve is intentionally a stub in the base commit. It will accept only
// local filesystem inputs once the library layer is implemented.
func (LocalFS) Resolve(ctx context.Context, input string) ([]Source, error) {
	return nil, nil
}
