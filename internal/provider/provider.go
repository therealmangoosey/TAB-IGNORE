// Package provider implements the source provider layer. Every provider is
// explicit, allow-listed, and configured through the registry. The adapters in
// this package never execute third-party JavaScript and never persist cookies.
package provider

import (
	"context"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

// Caps describes a provider's capabilities. NeedsJS is always false for the
// providers that ship in-tree; a provider that would need JS is rejected by
// the registry rather than silently executed.
type Caps struct {
	HasSearch      bool
	HasEpisodeEnum bool
	HasSubtitles   bool
	MultiAudio     bool
	Progressive    bool
	HLS            bool
	DASH           bool
	NeedsReferer   bool
	NeedsCookie    bool
	NeedsJS        bool
	BaseTTL        time.Duration
}

// ProbeResult is the cheap availability probe for a source.
type ProbeResult struct {
	OK        bool
	LatencyMS int64
	SizeBytes int64
	Quality   hermit.Quality
	Codec     hermit.Codec
	Note      string
}

// Provider is the core interface implemented by source adapters.
type Provider interface {
	ID() string
	Caps() Caps
	Resolve(ctx context.Context, ref hermit.Ref) ([]hermit.Source, error)
}

// Searcher is implemented by providers that can search their own catalog.
type Searcher interface {
	Search(ctx context.Context, query string, kind hermit.Kind) ([]hermit.Hit, error)
}

// Probber is implemented by providers that can probe sources cheaply.
type Probber interface {
	Probe(ctx context.Context, src hermit.Source) (ProbeResult, error)
}

// Subtitler is implemented by providers that expose subtitles.
type Subtitler interface {
	Subtitles(ctx context.Context, ref hermit.Ref) ([]Subtitle, error)
}

// Subtitle is one subtitle candidate.
type Subtitle struct {
	Language string `json:"language"`
	Label    string `json:"label"`
	URL      string `json:"url"`
	Format   string `json:"format"`
}

// HostStats is the per-host EWMA health record persisted by the registry.
type HostStats struct {
	Origin        string
	EWMASuccess   float64
	Samples       int
	MedianLatency int64
	LastOKAt      time.Time
	LastFailAt    time.Time
	BannedUntil   time.Time
}
