package srv

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/dms/upnpav"
	"github.com/therealmangoosey/TAB-IGNORE/internal/lib"
)

func TestDLNAMakeObjectUsesFileDateAndSize(t *testing.T) {
	root := t.TempDir()
	library := lib.New(root, "", "")
	d := &dlnaServer{library: library, name: "Hermit"}
	path := filepath.Join(root, "Show", "Episode.mp4")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2024, 7, 15, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, want, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	obj, err := d.makeObject("/Show/Episode.mp4", info, "127.0.0.1:8789")
	if err != nil {
		t.Fatal(err)
	}
	item, ok := obj.(upnpav.Item)
	if !ok {
		t.Fatalf("got %T, want upnpav.Item", obj)
	}
	if item.Date.Time.IsZero() {
		t.Fatal("DLNA date must not be zero")
	}
	if item.Date.Time.Year() != want.Year() || item.Date.Time.Month() != want.Month() || item.Date.Time.Day() != want.Day() {
		t.Fatalf("date = %s, want %s", item.Date.Time.Format("2006-01-02"), want.Format("2006-01-02"))
	}
	if len(item.Res) != 1 || item.Res[0].Size != 5 {
		t.Fatalf("resource size metadata incorrect: %+v", item.Res)
	}
}

func TestDLNABrowsePathRejectsEscape(t *testing.T) {
	d := &dlnaServer{library: lib.New(t.TempDir(), "", "")}
	if _, _, err := d.browsePath("/../../etc/passwd"); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}

func TestMediaMIME(t *testing.T) {
	tests := map[string]string{
		"episode.mp4":  "video/mp4",
		"episode.m4v":  "video/mp4",
		"episode.webm": "video/webm",
		"episode.mkv":  "video/x-matroska",
		"episode.ts":   "video/mp2t",
		"episode.txt":  "text/plain; charset=utf-8",
	}
	for path, want := range tests {
		if got := mediaMIME(path); got != want {
			t.Errorf("mediaMIME(%q) = %q, want %q", path, got, want)
		}
	}
}
