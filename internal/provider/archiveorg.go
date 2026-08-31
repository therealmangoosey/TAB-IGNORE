package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

// ArchiveOrg is a provider for Internet Archive public-domain and
// appropriately-licensed media using the documented advancedsearch and
// metadata APIs.
type ArchiveOrg struct {
	Client *http.Client
	Base   string
}

// NewArchiveOrg creates the provider.
func NewArchiveOrg(client *http.Client, base string) *ArchiveOrg {
	if base == "" {
		base = "https://archive.org"
	}
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &ArchiveOrg{Client: client, Base: strings.TrimRight(base, "/")}
}

// ID implements Provider.
func (a *ArchiveOrg) ID() string { return "archiveorg" }

// Caps implements Provider.
func (a *ArchiveOrg) Caps() Caps {
	return Caps{HasSearch: true, HasEpisodeEnum: false, Progressive: true, HLS: false, BaseTTL: 6 * time.Hour}
}

type archiveSearchResp struct {
	Response struct {
		Docs []struct {
			Identifier string `json:"identifier"`
			Title      string `json:"title"`
			Year       *int   `json:"year"`
			MediaType  string `json:"mediatype"`
		} `json:"docs"`
	} `json:"response"`
}

// Search queries the Internet Archive advancedsearch API.
func (a *ArchiveOrg) Search(ctx context.Context, q string, _ hermit.Kind) ([]hermit.Hit, error) {
	params := url.Values{}
	params.Set("q", fmt.Sprintf("%s AND mediatype:(movies)", q))
	params.Set("fl[]", "identifier")
	params.Set("fl[]", "title")
	params.Set("fl[]", "year")
	params.Set("rows", "30")
	params.Set("page", "1")
	params.Set("output", "json")
	u := a.Base + "/advancedsearch.php?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("archive.org search: HTTP %d", resp.StatusCode)
	}
	var body bytesBuffer
	if _, err := body.ReadFrom(resp.Body); err != nil {
		return nil, err
	}
	var out archiveSearchResp
	if err := json.Unmarshal(body.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("archive.org search json: %w", err)
	}
	var hits []hermit.Hit
	for _, d := range out.Response.Docs {
		year := 0
		if d.Year != nil {
			year = *d.Year
		}
		hits = append(hits, hermit.Hit{
			Ref:      hermit.Ref{Kind: hermit.KindMovie, IMDBID: d.Identifier, Title: d.Identifier, Provider: "archiveorg"},
			Title:    d.Title,
			Year:     year,
			Provider: "archiveorg",
			Kind:     hermit.KindMovie,
		})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Title < hits[j].Title })
	return hits, nil
}

type archiveMeta struct {
	Server string `json:"server"`
	Dir    string `json:"dir"`
	Files  []struct {
		Name   string      `json:"name"`
		Format string      `json:"format"`
		Size   json.Number `json:"size"`
		Source string      `json:"source"`
		Length json.Number `json:"length"`
	} `json:"files"`
}

// Resolve returns a direct download source for an archive item. ref.IMDBID is
// the archive identifier; ref.Title is used as a fallback search query.
func (a *ArchiveOrg) Resolve(ctx context.Context, ref hermit.Ref) ([]hermit.Source, error) {
	id := ref.IMDBID
	if id == "" {
		search, err := a.Search(ctx, ref.Title, ref.Kind)
		if err != nil {
			return nil, err
		}
		if len(search) == 0 {
			return nil, fmt.Errorf("archive.org: no items for %q", ref.Title)
		}
		id = search[0].Ref.IMDBID
	}
	metaURL := fmt.Sprintf("%s/metadata/%s", a.Base, url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metaURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("archive.org metadata: HTTP %d", resp.StatusCode)
	}
	var meta archiveMeta
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, fmt.Errorf("archive.org metadata json: %w", err)
	}
	var sources []hermit.Source
	for _, f := range meta.Files {
		format := strings.ToLower(f.Format)
		if !(format == "mp4" || format == "matroska" || format == "h.264" || format == "mpeg4") {
			continue
		}
		if f.Source == "original" || strings.HasSuffix(strings.ToLower(f.Name), ".mp4") || strings.HasSuffix(strings.ToLower(f.Name), ".mkv") {
			var size int64
			_ = f.Size.Int64()
			if n, err := f.Size.Int64(); err == nil {
				size = n
			}
			server := meta.Server
			if server == "" {
				server = "ia800000.us.archive.org"
			}
			fileURL := fmt.Sprintf("https://%s%s/%s", server, meta.Dir, f.Name)
			if meta.Dir == "" {
				fileURL = fmt.Sprintf("https://archive.org/download/%s/%s", url.PathEscape(id), f.Name)
			}
			sources = append(sources, hermit.Source{
				ID:        "archive-" + url.QueryEscape(id) + "-" + f.Name,
				Label:     f.Name,
				Provider:  "archiveorg",
				Kind:      hermit.TransportDirect,
				URL:       fileURL,
				Quality:   hermit.QualityAuto,
				Codec:     hermit.CodecUnknown,
				SizeBytes: size,
			})
		}
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("archive.org item %s has no supported media file", id)
	}
	return sources, nil
}

// Probe sends a Range request to the source.
func (a *ArchiveOrg) Probe(ctx context.Context, src hermit.Source) (ProbeResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return ProbeResult{OK: false, Note: err.Error()}, nil
	}
	req.Header.Set("Range", "bytes=0-65535")
	start := time.Now()
	resp, err := a.Client.Do(req)
	if err != nil {
		return ProbeResult{OK: false, Note: err.Error(), LatencyMS: time.Since(start).Milliseconds()}, nil
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 65536))
	ok := resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent
	return ProbeResult{OK: ok, LatencyMS: time.Since(start).Milliseconds(), Note: fmt.Sprintf("HTTP %d", resp.StatusCode)}, nil
}

// bytesBuffer is a small reader that keeps archive response parsing cheap.
type bytesBuffer struct {
	bytes []byte
}

func (b *bytesBuffer) ReadFrom(r io.Reader) (int64, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	b.bytes = data
	return int64(len(data)), nil
}

func (b *bytesBuffer) Bytes() []byte { return b.bytes }
