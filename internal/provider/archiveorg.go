package provider

import "context"

type ArchiveOrg struct{}

func (ArchiveOrg) Name() string { return "archiveorg" }

// Resolve remains intentionally unimplemented in the bootstrap commit. The
// finished adapter must use the documented Internet Archive APIs only.
func (ArchiveOrg) Resolve(ctx context.Context, input string) ([]Source, error) {
	return nil, nil
}
