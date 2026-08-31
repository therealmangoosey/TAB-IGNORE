package srv

import "testing"

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
