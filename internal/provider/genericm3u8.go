package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

// GenericM3U8 accepts a user-supplied media manifest or direct media URL. It
// performs no discovery and never follows a JS embed cascade.
type GenericM3U8 struct {
	Client *http.Client
}

// NewGenericM3U8 creates the provider.
func NewGenericM3U8(client *http.Client) *GenericM3U8 {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &GenericM3U8{Client: client}
}

// ID implements Provider.
func (g *GenericM3U8) ID() string { return "genericm3u8" }

// Caps implements Provider.
func (g *GenericM3U8) Caps() Caps {
	return Caps{HasSearch: false, HasEpisodeEnum: false, Progressive: true, HLS: true, BaseTTL: 1 * time.Hour}
}

// Resolve validates and returns a user-supplied URL. ref.Title holds the URL.
func (g *GenericM3U8) Resolve(ctx context.Context, ref hermit.Ref) ([]hermit.Source, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	raw := strings.TrimSpace(ref.Title)
	if raw == "" {
		raw = ref.IMDBID
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid media URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "file" {
		return nil, fmt.Errorf("media URL must be http, https, or file")
	}
	if u.Host == "" && u.Scheme != "file" {
		return nil, fmt.Errorf("media URL has no host")
	}
	kind := hermit.TransportDirect
	if strings.HasSuffix(strings.ToLower(u.Path), ".m3u8") {
		kind = hermit.TransportHLS
	}
	return []hermit.Source{
		{
			ID:       "generic-" + shortID(raw),
			Label:    u.Path,
			Provider: "genericm3u8",
			Kind:     kind,
			URL:      raw,
			Quality:  hermit.QualityAuto,
			Codec:    hermit.CodecUnknown,
		},
	}, nil
}

// Probe issues a ranged request to confirm the source is reachable.
func (g *GenericM3U8) Probe(ctx context.Context, src hermit.Source) (ProbeResult, error) {
	if strings.HasPrefix(src.URL, "file://") {
		return ProbeResult{OK: true, Note: "local file"}, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return ProbeResult{OK: false, Note: err.Error()}, nil
	}
	req.Header.Set("Range", "bytes=0-65535")
	start := time.Now()
	resp, err := g.Client.Do(req)
	if err != nil {
		return ProbeResult{OK: false, Note: err.Error(), LatencyMS: time.Since(start).Milliseconds()}, nil
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 65536))
	ok := resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent
	return ProbeResult{OK: ok, LatencyMS: time.Since(start).Milliseconds(), Note: fmt.Sprintf("HTTP %d", resp.StatusCode)}, nil
}

func shortID(s string) string {
	var b [8]byte
	for i := 0; i < len(b) && i < len(s); i++ {
		b[i] = s[i]
	}
	return string(b[:])
}
