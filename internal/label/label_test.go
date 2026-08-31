package label

import "testing"

func TestFilename(t *testing.T) {
	got := Filename("Severance", 1, 1, "Good News About Tilly", "mp4")
	if got != "Severance - S01E01 - Good News About Tilly.mp4" {
		t.Fatalf("unexpected filename: %s", got)
	}
}

func TestFilenameSanitizes(t *testing.T) {
	got := Filename("A/B", 2, 3, "Title: X", "MP4")
	want := "A-B - S02E03 - Title - X.mp4"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestClean(t *testing.T) {
	if got := Clean("  A  B / C  "); got != "A B - C" {
		t.Fatalf("clean: %q", got)
	}
}

func TestMovieFilename(t *testing.T) {
	if got := MovieFilename("Titanic", 1997, "mkv"); got != "Titanic (1997).mkv" {
		t.Fatalf("movie: %q", got)
	}
}

func TestLineRedacts(t *testing.T) {
	red := Redact("https://api.example.com/path", map[string]string{"api.example.com": "api.example.com"})
	if red == "https://api.example.com/path" {
		t.Fatalf("expected hostname to be redacted")
	}
	if red != "https://h1/path" {
		t.Fatalf("redacted: %q", red)
	}
}
