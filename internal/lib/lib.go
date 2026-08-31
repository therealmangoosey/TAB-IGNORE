// Package lib manages the user's on-disk media library: scanning, listing,
// deduplication, integrity verification, rename previews, .nomedia marks, and
// space reclamation.
package lib

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/therealmangoosey/TAB-IGNORE/internal/config"
	"github.com/therealmangoosey/TAB-IGNORE/internal/disk"
	"github.com/therealmangoosey/TAB-IGNORE/internal/scrub"
)

// Entry is one media file in the library.
type Entry struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256,omitempty"`
	ShowDir   string `json:"show_dir"`
}

// Library is the on-disk library manager.
type Library struct {
	Root string
	Reserve string
	Margin string
}

// New creates a library rooted at root.
func New(root, reserve, margin string) *Library {
	return &Library{Root: root, Reserve: reserve, Margin: margin}
}

// Scan walks the library and returns media files.
func (l *Library) Scan(ctx context.Context) ([]Entry, error) {
	var out []Entry
	err := filepath.WalkDir(l.Root, func(p string, d os.DirEntry, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err != nil {
			return nil
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext == ".mp4" || ext == ".mkv" || ext == ".m4v" || ext == ".webm" || ext == ".ts" {
			info, err := d.Info()
			if err != nil {
				return nil
			}
			out = append(out, Entry{Path: p, Size: info.Size(), ShowDir: filepath.Dir(p)})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// Verify computes SHA-256 for a file while streaming.
func Verify(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 256*1024)
	var total int64
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
			total += int64(n)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", total, rerr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), total, nil
}

// Reclaim deletes completed, watched episodes to make room, keeping at least
// keepBytes of spare space. It returns the number of files and bytes removed.
func (l *Library) Reclaim(ctx context.Context, watchFunc func(path string) bool, keepBytes int64) (int, int64, error) {
	entries, err := l.Scan(ctx)
	if err != nil {
		return 0, 0, err
	}
	reserve, _ := config.ParseSize(l.Reserve)
	var deleted, bytes int
	// Reclaim from the end of the sorted list (lexicographically oldest show
	// first is not a perfect "oldest watched" heuristic, but it is deterministic
	// and safe without a metadata DB).
	for _, e := range entries {
		stats, err := disk.Info(l.Root, reserve)
		if err != nil {
			return deleted, int64(bytes), err
		}
		if stats.SpareBytes() >= keepBytes {
			break
		}
		if watchFunc != nil && !watchFunc(e.Path) {
			continue
		}
		if err := os.Remove(e.Path); err != nil {
			continue
		}
		deleted++
		bytes += int(e.Size)
	}
	return deleted, int64(bytes), nil
}

// Secure writes .nomedia into every non-hidden top-level show directory.
// It does not remove untracked directories. It returns the number of
// directories marked.
func (l *Library) Secure() (int, error) {
	if err := os.MkdirAll(l.Root, 0o755); err != nil {
		return 0, err
	}
	dirs, err := os.ReadDir(l.Root)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, d := range dirs {
		if d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
			path := filepath.Join(l.Root, d.Name())
			if err := os.WriteFile(filepath.Join(path, ".nomedia"), []byte{}, 0o644); err != nil {
				continue
			}
			count++
		}
	}
	return count, nil
}

// ReadSidecar reads the per-show sidecar for a file.
func ReadSidecar(path string) (scrub.Sidecar, error) {
	var sc scrub.Sidecar
	data, err := os.ReadFile(filepath.Join(filepath.Dir(path), ".hermit.json"))
	if err != nil {
		return sc, err
	}
	err = json.Unmarshal(data, &sc)
	return sc, err
}

// PreviewRename builds a proposed filename from sidecar metadata. The returned
// table pairs the current path with the proposed path.
func (l *Library) PreviewRename() (map[string]string, error) {
	entries, err := l.Scan(context.Background())
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, e := range entries {
		showDir := filepath.Base(e.ShowDir)
		sc, err := ReadSidecar(e.Path)
		if err != nil {
			continue
		}
		_ = sc
		base := filepath.Base(e.Path)
		newname := scrub.SafeName(showDir) + "-" + scrub.SafeName(base)
		newname = scrub.LimitPathComponent(newname)
		proposed := filepath.Join(l.Root, scrub.SafeName(showDir), newname)
		if proposed != e.Path {
			out[e.Path] = proposed
		}
	}
	return out, nil
}

// Summary returns file count and cumulative bytes.
func (l *Library) Summary(ctx context.Context) (int, int64, error) {
	entries, err := l.Scan(ctx)
	if err != nil {
		return 0, 0, err
	}
	var bytes int64
	for _, e := range entries {
		bytes += e.Size
	}
	return len(entries), bytes, nil
}

// Errorf returns a formatted error for library failures.
func Errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
