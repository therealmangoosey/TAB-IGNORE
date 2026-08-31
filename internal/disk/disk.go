// Package disk implements the spare-storage rule used by the queue and TUI.
// hermit never reports raw free space for capacity decisions; it always
// subtracts a configurable reserve.
package disk

import (
	"fmt"
	"os"
	"path/filepath"
)

// Stats describes a filesystem.
type Stats struct {
	Path         string `json:"path"`
	FreeBytes    int64  `json:"free_bytes"`
	TotalBytes   int64  `json:"total_bytes"`
	ReserveBytes int64  `json:"reserve_bytes"`
}

// SpareBytes is free bytes minus the reserve.
func (s Stats) SpareBytes() int64 {
	n := s.FreeBytes - s.ReserveBytes
	if n < 0 {
		return 0
	}
	return n
}

// Info returns filesystem stats for a path, including the configured reserve.
func Info(path string, reserve int64) (Stats, error) {
	free, total, err := statfs(path)
	if err != nil {
		return Stats{}, err
	}
	return Stats{Path: path, FreeBytes: free, TotalBytes: total, ReserveBytes: reserve}, nil
}

// Ensure creates the path and verifies it is writable.
func Ensure(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			if mkErr := os.MkdirAll(path, 0o755); mkErr != nil {
				return fmt.Errorf("create %s: %w", path, mkErr)
			}
		} else {
			return fmt.Errorf("stat %s: %w", path, err)
		}
	} else if !info.IsDir() {
		return fmt.Errorf("%s exists and is not a directory", path)
	}
	probe := filepath.Join(path, ".hermit-write-test")
	f, err := os.Create(probe)
	if err != nil {
		return fmt.Errorf("write to %s: %w", path, err)
	}
	f.Close()
	os.Remove(probe)
	return nil
}

// Fits reports whether a download of bytes with margin fits in spare space.
func Fits(path string, bytes, reserve, margin int64) (bool, error) {
	free, _, err := statfs(path)
	if err != nil {
		return false, err
	}
	spare := free - reserve
	return spare >= bytes+margin, nil
}
