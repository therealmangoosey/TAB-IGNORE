package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/internal/config"
	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

// The in-tree provider allow-list. Adapters outside this list are never
// instantiated by the registry, regardless of config.
var allowedIDs = map[string]bool{
	"localfs":     true,
	"archiveorg":  true,
	"genericm3u8": true,
	"debrid":      true,
}

// Registry owns explicitly enabled providers and their health state.
type Registry struct {
	mu        sync.RWMutex
	items     map[string]Provider
	configs   map[string]config.ProviderConfig
	client    *http.Client
	hosts     map[string]HostStats
}

// NewRegistry builds providers from config using a shared hardened client.
// Unknown or unofficial provider IDs are ignored with a logged diagnostic.
func NewRegistry(cfg config.Config) (*Registry, error) {
	r := &Registry{
		items:   map[string]Provider{},
		configs: map[string]config.ProviderConfig{},
		client:  &http.Client{Timeout: 20 * time.Second},
		hosts:   map[string]HostStats{},
	}
	for _, p := range cfg.Providers.Entries {
		if !p.Enabled || !allowedIDs[p.ID] {
			continue
		}
		switch p.ID {
		case "localfs":
			prov, err := NewLocalFS(cfg.Library.Path)
			if err != nil {
				return nil, err
			}
			r.items[p.ID] = prov
		case "archiveorg":
			r.items[p.ID] = NewArchiveOrg(r.client, p.Base)
		case "genericm3u8":
			r.items[p.ID] = NewGenericM3U8(r.client)
		case "debrid":
			r.items[p.ID] = NewDebrid(r.client, p.Base, p.Token, p.Headers)
		}
		r.configs[p.ID] = p
	}
	return r, nil
}

// DefaultRegistry returns the registry used when no config is available.
func DefaultRegistry(libraryPath string) (*Registry, error) {
	cfg := config.Default()
	cfg.Library.Path = libraryPath
	return NewRegistry(cfg)
}

// Names returns enabled providers in stable order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []string
	for id := range r.items {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Base returns the configured base URL for a provider.
func (r *Registry) Base(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if c, ok := r.configs[name]; ok {
		return c.Base
	}
	return ""
}

// Get returns a provider by ID.
func (r *Registry) Get(name string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.items[name]
	if !ok {
		return nil, fmt.Errorf("provider %q is not enabled", name)
	}
	return p, nil
}

// Resolve resolves a ref with one provider.
func (r *Registry) Resolve(ctx context.Context, name string, ref hermit.Ref) ([]hermit.Source, error) {
	p, err := r.Get(name)
	if err != nil {
		return nil, err
	}
	return p.Resolve(ctx, ref)
}

// ResolveAll resolves a ref through every enabled provider and scores the
// result. Provider failures are returned separately so callers can present
// partial availability instead of failing the whole request.
func (r *Registry) ResolveAll(ctx context.Context, ref hermit.Ref) ([]hermit.Source, map[string]error) {
	r.mu.RLock()
	names := make([]string, 0, len(r.items))
	for name := range r.items {
		names = append(names, name)
	}
	r.mu.RUnlock()

	var out []hermit.Source
	errs := map[string]error{}
	for _, name := range names {
		p, err := r.Get(name)
		if err != nil {
			errs[name] = err
			continue
		}
		srcs, err := p.Resolve(ctx, ref)
		if err != nil {
			errs[name] = err
			continue
		}
		for i := range srcs {
			srcs[i].Score = scoreSource(srcs[i], r)
			out = append(out, srcs[i])
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, errs
}

// Search searches all providers implementing Searcher.
func (r *Registry) Search(ctx context.Context, q string, kind hermit.Kind) ([]hermit.Hit, error) {
	var out []hermit.Hit
	for _, name := range r.Names() {
		p, _ := r.Get(name)
		s, ok := p.(Searcher)
		if !ok {
			continue
		}
		hits, err := s.Search(ctx, q, kind)
		if err != nil {
			continue
		}
		out = append(out, hits...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}

// Probe probes a source through its provider and updates host stats.
func (r *Registry) Probe(ctx context.Context, src hermit.Source) (ProbeResult, error) {
	p, err := r.Get(src.Provider)
	if err != nil {
		return ProbeResult{OK: false, Note: err.Error()}, err
	}
	prob, ok := p.(Probber)
	if !ok {
		prob = probeFallback(r.client, src)
	}
	res, err := prob.Probe(ctx, src)
	r.recordHost(src.URL, res.OK, res.LatencyMS)
	return res, err
}

// HostStats returns host health records.
func (r *Registry) HostStats() []HostStats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]HostStats, 0, len(r.hosts))
	for _, h := range r.hosts {
		out = append(out, h)
	}
	return out
}

func probeFallback(client *http.Client, src hermit.Source) Probber {
	return &fallbackProber{client: client, src: src}
}

type fallbackProber struct {
	client *http.Client
	src    hermit.Source
}

func (p *fallbackProber) Probe(ctx context.Context, src hermit.Source) (ProbeResult, error) {
	return genericProbe(ctx, p.client, src)
}

func (r *Registry) recordHost(rawURL string, ok bool, latency int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	host := originOf(rawURL)
	if host == "" {
		return
	}
	h := r.hosts[host]
	if h.Origin == "" {
		h.Origin = host
		h.EWMASuccess = 0.7
	}
	alpha := 0.2
	score := 0.0
	if ok {
		score = 1
	}
	h.EWMASuccess = alpha*score + (1-alpha)*h.EWMASuccess
	h.Samples++
	if ok {
		h.LastOKAt = time.Now()
	} else {
		h.LastFailAt = time.Now()
	}
	if latency > 0 {
		if h.MedianLatency == 0 {
			h.MedianLatency = latency
		} else {
			h.MedianLatency = (h.MedianLatency + latency) / 2
		}
	}
	if !ok && h.Samples >= 3 && h.EWMASuccess < 0.25 {
		h.BannedUntil = time.Now().Add(24 * time.Hour)
	}
	r.hosts[host] = h
}

func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}
