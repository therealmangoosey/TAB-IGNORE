// Package fetch implements the hardened HTTP transport, ranged downloads,
// HLS manifest handling, bounded concurrency, and streaming SHA-256 integrity
// checks used by the queue.
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

// AllowListTransport enforces an explicit origin allow-list. Every redirect is
// re-checked, so a redirect cannot silently jump to an unapproved host.
type AllowListTransport struct {
	Base     http.RoundTripper
	Allowed  map[string]bool
	AllowAll bool
}

// RoundTrip implements http.RoundTripper.
func (t *AllowListTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.AllowAll && !t.allow(req.URL.Scheme+"://"+req.URL.Host) {
		return nil, fmt.Errorf("origin %q is not in the hermit allow-list", req.URL.Host)
	}
	if t.Base == nil {
		t.Base = http.DefaultTransport
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

// NewClient builds a hardened client with bounded connection reuse.
// When `hmt vpn up` is active, only sockets created by this client are marked
// for the VPN routing table. Other device/application traffic is unaffected.
func NewClient(allowed []string) *http.Client {
	allowedMap := map[string]bool{}
	for _, a := range allowed {
		if a != "" {
			allowedMap[a] = true
		}
	}
	dialContext := vpn.DialContext
	tr := &http.Transport{
		MaxIdleConns:        8,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     30 * time.Second,
		DialContext:         dialContext,
	}
	return &http.Client{
		Transport: &RedirectCheck{Next: &AllowListTransport{Base: tr, Allowed: allowedMap}},
		Timeout:   4 * time.Minute,
	}
}

// RedirectCheck validates each redirect hop against the same allow-list.
type RedirectCheck struct {
	Next http.RoundTripper
}

// RoundTrip validates redirect location and delegates.
func (r *RedirectCheck) RoundTrip(req *http.Request) (*http.Response, error) {
	if sp, ok := r.Next.(*AllowListTransport); ok {
		if !sp.allow(req.URL.Scheme + "://" + req.URL.Host) {
			return nil, fmt.Errorf("origin %q is not in the hermit allow-list", req.URL.Host)
		}
	}
	return r.Next.RoundTrip(req)
}

// BoundedRate is a tiny token-bucket rate limiter with no extra dependency.
type BoundedRate struct {
	mu       sync.Mutex
	maxBytes int64
	tokens   float64
	last     time.Time
}

// NewBoundedRate creates a rate limiter for max bytes per second.
func NewBoundedRate(maxBytes int64) *BoundedRate {
	if maxBytes <= 0 {
		maxBytes = 4 * 1024 * 1024
	}
	return &BoundedRate{maxBytes: maxBytes, tokens: float64(maxBytes), last: time.Now()}
}

// Wait blocks until n bytes can be consumed.
func (b *BoundedRate) Wait(n int64) {
	if n <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.last).Seconds()
	b.last = now
	b.tokens += elapsed * float64(b.maxBytes)
	if b.tokens > float64(b.maxBytes) {
		b.tokens = float64(b.maxBytes)
	}
	if b.tokens < float64(n) {
		need := (float64(n) - b.tokens) / float64(b.maxBytes)
		time.Sleep(time.Duration(need * float64(time.Second)))
		b.last = time.Now()
		b.tokens = 0
		return
	}
	b.tokens -= float64(n)
}

// HostForURL returns a URL's host.
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

// Origins extracts all hosts from a list of source URLs.
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
