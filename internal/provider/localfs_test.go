package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

func TestLocalFSResolveFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "movie.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	prov, err := NewLocalFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	srcs, err := prov.Resolve(context.Background(), hermit.Ref{Title: filepath.Join(dir, "movie.mp4")})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(srcs) != 1 {
		t.Fatalf("expected 1 source, got %d", len(srcs))
	}
	if srcs[0].Kind != hermit.TransportDirect {
		t.Fatalf("kind: %s", srcs[0].Kind)
	}
}

func TestLocalFSSearch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "severance_s01e01.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	prov, err := NewLocalFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := prov.Search(context.Background(), "severance", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
}

func TestLocalFSPathEscapeRejected(t *testing.T) {
	dir := t.TempDir()
	prov, err := NewLocalFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = prov.Resolve(context.Background(), hermit.Ref{Title: filepath.Join(dir, "..", "escape.mp4")})
	if err == nil {
		t.Fatal("expected path escape error")
	}
}
