// Package fetch implements the hardened HTTP transport, ranged downloads, HLS manifest handling,
// bounded concurrency, and streaming SHA-256 integrity checks used by the queue.
package fetch

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/internal/vpn"
)

type AllowListTransport struct {
	Base     http.RoundTripper
	Allowed  map[string]bool
	AllowAll bool
}

func (t *AllowListTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.AllowAll && !t.allow(req.URL.Scheme+"://"+req.URL.Host) {
		return nil, fmt.Errorf("origin %q is not in the hermit allow-list", req.URL.Host)
	}
	return t.Base.RoundTrip(req)
}

func (t *AllowListTransport) allow(origin string) bool {
	if t.AllowAll {
		return true
	}
	for allowed := range t.Allowed {
		if strings.EqualFold(strings.TrimRight(allowed, "/"), strings.TrimRight(origin, "/")) {
			return true
		}
	}
	return false
}

func NewClient(allowed []string) *http.Client {
	allowedMap := map[string]bool{}
	for _, a := range allowed {
		if a != "" {
			allowedMap[a] = true
		}
	}
	tr := &http.Transport{
		MaxIdleConns:        8,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     30 * time.Second,
		DialContext:         vpn.DialContext,
	}
	return &http.Client{
		Transport: &RedirectCheck{Next: &AllowListTransport{
			Base:     tr,
			Allowed:  allowedMap,
			AllowAll: len(allowedMap) == 0,
		}},
		Timeout: 4 * time.Minute,
	}
}

type RedirectCheck struct {
	Next http.RoundTripper
}

func (r *RedirectCheck) RoundTrip(req *http.Request) (*http.Response, error) {
	if sp, ok := r.Next.(*AllowListTransport); ok {
		if !sp.allow(req.URL.Scheme + "://" + req.URL.Host) {
			return nil, fmt.Errorf("origin %q is not in the hermit allow-list", req.URL.Host)
		}
	}
	return r.Next.RoundTrip(req)
}

type BoundedRate struct {
	mu       sync.Mutex
	maxBytes int64
	tokens   float64
	last     time.Time
}

func NewBoundedRate(maxBytes int64) *BoundedRate {
	if maxBytes <= 0 {
		maxBytes = 4 * 1024 * 1024
	}
	return &BoundedRate{maxBytes: maxBytes, tokens: float64(maxBytes), last: time.Now()}
}

func (b *BoundedRate) Wait(n int64) {
	if n <= 0 {
		return
	}
	for n > 0 {
		b.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(b.last).Seconds()
		if elapsed > 0 {
			b.tokens += elapsed * float64(b.maxBytes)
			if b.tokens > float64(b.maxBytes) {
				b.tokens = float64(b.maxBytes)
			}
			b.last = now
		}
		chunk := n
		if chunk > b.maxBytes {
			chunk = b.maxBytes
		}
		if b.tokens >= float64(chunk) {
			b.tokens -= float64(chunk)
			b.mu.Unlock()
			n -= chunk
			continue
		}
		need := (float64(chunk) - b.tokens) / float64(b.maxBytes)
		b.mu.Unlock()
		if need > 0 {
			time.Sleep(time.Duration(need * float64(time.Second)))
		}
	}
}

func HostForURL(raw string) string {
	if strings.HasPrefix(raw, "file://") {
		return "local"
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

func Origins(sources []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range sources {
		if h := HostForURL(raw); h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}
