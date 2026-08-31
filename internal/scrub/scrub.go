// Package scrub normalizes filenames and container metadata so the library on
// disk looks like a library rather than a download dump. It deliberately does
// not conceal network traffic; it only keeps user-owned files self-contained.
package scrub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/therealmangoosey/TAB-IGNORE/internal/label"
)

// Sidecar is the tiny per-show metadata file.
type Sidecar struct {
	TMDBID    int               `json:"tmdb_id,omitempty"`
	AniListID int               `json:"anilist_id,omitempty"`
	Types     string            `json:"type,omitempty"`
	Episodes  map[string]string `json:"episodes"`
}

// SafeName returns a filesystem-safe component name.
func SafeName(s string) string {
	return label.Clean(s)
}

// LimitPathComponent truncates a component to fit exFAT/Android limits.
func LimitPathComponent(name string) string {
	r := []rune(name)
	if len(r) <= 120 {
		return name
	}
	cut := 120
	for cut > 0 && r[cut-1] != ' ' {
		cut--
	}
	if cut <= 0 {
		cut = 120
	}
	return strings.TrimSpace(string(r[:cut]))
}

// Line returns the TUI/log label for a row.
func Line(show string, season, episode int, episodeTitle string) string {
	return label.Line(show, season, episode, episodeTitle)
}

// WriteSidecar writes a minimal .hermit.json next to a downloaded episode.
func WriteSidecar(file string, mediaID int64, sha256 string) error {
	dir := filepath.Dir(file)
	sc := Sidecar{
		Episodes: map[string]string{},
	}
	sc.Episodes[filepath.Base(file)] = sha256
	path := filepath.Join(dir, ".hermit.json")
	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ValidateNoSourceStrings is a lightweight test helper used by the library and
// CI to assert that a label contains no provider host or URL markers.
func ValidateNoSourceStrings(s string, markers []string) error {
	lower := strings.ToLower(s)
	for _, m := range markers {
		if m != "" && strings.Contains(lower, strings.ToLower(m)) {
			return &SourceStringError{Marker: m}
		}
	}
	return nil
}

// SourceStringError describes a provider marker that leaked into a label.
type SourceStringError struct {
	Marker string
}

func (e *SourceStringError) Error() string {
	return "source marker found in label: " + e.Marker
}
