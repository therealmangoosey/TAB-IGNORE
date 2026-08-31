package provider

import "context"

type GenericM3U8 struct{}

func (GenericM3U8) Name() string { return "genericm3u8" }

// Resolve is a placeholder for a user-supplied media manifest URL. Validation
// and allow-list enforcement belong in the fetch layer before network access.
func (GenericM3U8) Resolve(ctx context.Context, input string) ([]Source, error) {
	return nil, nil
}
