package disk

import (
	"path/filepath"
	"testing"
)

func TestInfoIncludesReserve(t *testing.T) {
	stats, err := Info(filepath.Join(t.TempDir(), "missing"), 123)
	if err == nil {
		t.Fatal("expected stat error for missing path")
	}
	_ = stats

	dir := t.TempDir()
	stats, err = Info(dir, 123)
	if err != nil {
		t.Fatal(err)
	}
	if stats.ReserveBytes != 123 {
		t.Fatalf("reserve = %d, want 123", stats.ReserveBytes)
	}
	if stats.SpareBytes() != stats.FreeBytes-123 && stats.FreeBytes > 123 {
		t.Fatalf("spare does not subtract reserve: %+v", stats)
	}
}
