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

// Debrid is the opt-in provider for a user-owned cloud drive direct-download
// link. It does not implement any particular debrid provider API; the server
// URL and optional bearer token come entirely from the user's config, and the
// link must already be a direct media URL on the user's own storage.
type Debrid struct {
	Client  *http.Client
	Base    string
	Token   string
	Headers map[string]string
}

// NewDebrid creates the provider.
func NewDebrid(client *http.Client, base, token string, headers map[string]string) *Debrid {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if headers == nil {
		headers = map[string]string{}
	}
	return &Debrid{Client: client, Base: strings.TrimRight(base, "/"), Token: token, Headers: headers}
}

// ID implements Provider.
func (d *Debrid) ID() string { return "debrid" }

// Caps implements Provider.
func (d *Debrid) Caps() Caps {
	return Caps{HasSearch: false, HasEpisodeEnum: false, Progressive: true, HLS: true, BaseTTL: 30 * time.Minute}
}

// Resolve returns the user-supplied direct URL. ref.Title is the download URL.
func (d *Debrid) Resolve(ctx context.Context, ref hermit.Ref) ([]hermit.Source, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	raw := strings.TrimSpace(ref.Title)
	if raw == "" && d.Base != "" {
		raw = d.Base
	}
	if raw == "" {
		return nil, fmt.Errorf("debrid URL is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("debrid URL must be http or https")
	}
	kind := hermit.TransportDirect
	if strings.HasSuffix(strings.ToLower(u.Path), ".m3u8") {
		kind = hermit.TransportHLS
	}
	return []hermit.Source{
		{
			ID:       "debrid-" + shortID(raw),
			Label:    "debrid download",
			Provider: "debrid",
			Kind:     kind,
			URL:      raw,
			Quality:  hermit.QualityAuto,
			Codec:    hermit.CodecUnknown,
		},
	}, nil
}

// Probe sends a range request.
func (d *Debrid) Probe(ctx context.Context, src hermit.Source) (ProbeResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return ProbeResult{OK: false, Note: err.Error()}, nil
	}
	for k, v := range d.Headers {
		req.Header.Set(k, v)
	}
	if d.Token != "" {
		req.Header.Set("Authorization", "Bearer "+d.Token)
	}
	req.Header.Set("Range", "bytes=0-65535")
	start := time.Now()
	resp, err := d.Client.Do(req)
	if err != nil {
		return ProbeResult{OK: false, Note: err.Error(), LatencyMS: time.Since(start).Milliseconds()}, nil
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 65536))
	ok := resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent
	return ProbeResult{OK: ok, LatencyMS: time.Since(start).Milliseconds(), Note: http.StatusText(resp.StatusCode)}, nil
}
