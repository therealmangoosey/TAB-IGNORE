package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

// LocalFS exposes files already owned by the user on the device or a mounted
// storage volume. It never follows symlinks outside its root and it never
// resolves remote URLs.
type LocalFS struct {
	Root string
}

var mediaExts = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true, ".mov": true, ".webm": true, ".ts": true, ".m3u8": true,
}

// NewLocalFS creates a provider rooted at root.
func NewLocalFS(root string) (*LocalFS, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("localfs root is empty")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("localfs root: %w", err)
	}
	return &LocalFS{Root: root}, nil
}

// ID implements Provider.
func (l *LocalFS) ID() string { return "localfs" }

// Caps implements Provider.
func (l *LocalFS) Caps() Caps {
	return Caps{HasSearch: true, HasEpisodeEnum: true, Progressive: true, HLS: true}
}

// Resolve accepts a filesystem path in ref.Title and returns one source, or
// scans the provided directory for media files.
func (l *LocalFS) Resolve(ctx context.Context, ref hermit.Ref) ([]hermit.Source, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	path := ref.Title
	if path == "" && ref.IMDBID != "" {
		path = ref.IMDBID
	}
	if path == "" {
		path = l.Root
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(l.Root, path)
	}
	rel, err := filepath.Rel(l.Root, path)
	if err != nil {
		return nil, fmt.Errorf("resolve localfs path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("path escapes localfs root")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("localfs stat: %w", err)
	}
	if info.IsDir() {
		var files []string
		err := filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if mediaExts[strings.ToLower(filepath.Ext(d.Name()))] {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		sort.Strings(files)
		out := make([]hermit.Source, 0, len(files))
		for _, f := range files {
			out = append(out, l.fileSource(f))
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("no media files in %s", path)
		}
		return out, nil
	}
	if !mediaExts[strings.ToLower(filepath.Ext(path))] {
		return nil, fmt.Errorf("%s is not a supported media file", filepath.Base(path))
	}
	return []hermit.Source{l.fileSource(path)}, nil
}

func (l *LocalFS) fileSource(path string) hermit.Source {
	size := int64(0)
	if info, err := os.Stat(path); err == nil {
		size = info.Size()
	}
	kind := hermit.TransportDirect
	if strings.HasSuffix(strings.ToLower(path), ".m3u8") {
		kind = hermit.TransportHLS
	}
	rel := path
	if abs, err := filepath.Abs(path); err == nil {
		rel = abs
	}
	rel, _ = filepath.Rel(l.Root, rel)
	raw, _ := json.Marshal(map[string]string{"path": path})
	return hermit.Source{
		ID:        "localfs-" + sanitizeID(rel),
		Label:     filepath.Base(path),
		Provider:  l.ID(),
		Kind:      kind,
		URL:       "file://" + filepath.ToSlash(path),
		Quality:   hermit.QualityAuto,
		Codec:     hermit.CodecUnknown,
		SizeBytes: size,
		Raw:       string(raw),
	}
}

// Search scans the library root for filenames matching query.
func (l *LocalFS) Search(ctx context.Context, query string, _ hermit.Kind) ([]hermit.Hit, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var hits []hermit.Hit
	err := filepath.WalkDir(l.Root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !mediaExts[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}
		name := strings.ToLower(filepath.Base(p))
		if q != "" && !strings.Contains(name, q) {
			return nil
		}
		ref := hermit.Ref{Kind: hermit.KindTV, IMDBID: p, Title: p, Provider: "localfs"}
		hits = append(hits, hermit.Hit{Ref: ref, Title: filepath.Base(p), Provider: "localfs", Kind: hermit.KindTV})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Title < hits[j].Title })
	return hits, nil
}

// Probe stat's a local file and reports success.
func (l *LocalFS) Probe(ctx context.Context, src hermit.Source) (ProbeResult, error) {
	select {
	case <-ctx.Done():
		return ProbeResult{}, ctx.Err()
	default:
	}
	path := src.URL
	path = strings.TrimPrefix(path, "file://")
	if info, err := os.Stat(path); err != nil {
		return ProbeResult{OK: false, Note: err.Error()}, nil
	} else {
		return ProbeResult{OK: true, SizeBytes: info.Size(), Note: "local"}, nil
	}
}

func sanitizeID(s string) string {
	s = strings.ToLower(s)
	s = strings.Join(strings.Fields(s), "-")
	s = strings.ReplaceAll(s, string(filepath.Separator), "-")
	return strings.Trim(s, "-")
}
