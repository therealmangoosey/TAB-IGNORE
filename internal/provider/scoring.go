package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

// scoreSource ranks a source for the low-RAM, no-AV1 target device.
func scoreSource(src hermit.Source, r *Registry) float64 {
	quality := 0.0
	switch src.Quality {
	case hermit.Quality1080:
		quality = 1.0
	case hermit.Quality720:
		quality = 0.7
	default:
		quality = 0.5
	}

	codec := 0.0
	switch src.Codec {
	case hermit.CodecHEVC:
		codec = 1.0
	case hermit.CodecAVC:
		codec = 0.9
	case hermit.CodecAV1:
		codec = -0.15
	default:
		codec = 0.5
	}

	latency := float64(src.LatencyMS)
	if latency > 2000 {
		latency = 2000
	}
	latencyScore := 1.0 - latency/2000.0
	if latency == 0 {
		latencyScore = 0.5
	}

	host := 0.0
	if stats := r.healthFor(src.URL); stats.Samples > 0 {
		if !stats.BannedUntil.IsZero() && time.Now().Before(stats.BannedUntil) {
			return -1.0
		}
		host = stats.EWMASuccess
	} else {
		host = 0.7
	}

	size := 0.0
	if src.SizeBytes > 0 {
		size = 1.0
	}

	return 0.45*quality + 0.20*codec + 0.15*host + 0.10*latencyScore + 0.05*size
}

func (r *Registry) healthFor(rawURL string) HostStats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.hosts[originOf(rawURL)]
	if !ok {
		return HostStats{}
	}
	return h
}

// genericProbe sends a ranged request to an arbitrary http source.
func genericProbe(ctx context.Context, client *http.Client, src hermit.Source) (ProbeResult, error) {
	if src.URL == "" {
		return ProbeResult{OK: false, Note: "empty url"}, nil
	}
	if strings.HasPrefix(src.URL, "file://") {
		return ProbeResult{OK: true, Note: "local"}, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return ProbeResult{OK: false, Note: err.Error()}, nil
	}
	req.Header.Set("Range", "bytes=0-65535")
	if src.Referer != "" {
		req.Header.Set("Referer", src.Referer)
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return ProbeResult{OK: false, Note: err.Error(), LatencyMS: time.Since(start).Milliseconds()}, nil
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 65536))
	ok := resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent
	return ProbeResult{OK: ok, LatencyMS: time.Since(start).Milliseconds(), Note: http.StatusText(resp.StatusCode)}, nil
}
