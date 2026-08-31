package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSize(t *testing.T) {
	tests := map[string]int64{
		"4MiB":  4 << 20,
		"3GB":   3_000_000_000,
		"512MB": 512_000_000,
		"1024":  1024,
		"1MiB":  1 << 20,
	}
	for in, want := range tests {
		got, err := ParseSize(in)
		if err != nil {
			t.Fatalf("ParseSize(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("ParseSize(%q)=%d, want %d", in, got, want)
		}
	}
}

func TestParseDuration(t *testing.T) {
	if d, err := ParseDuration("24h"); err != nil || d.Hours() != 24 {
		t.Fatalf("24h: %v %v", d, err)
	}
	if d, err := ParseDuration("5"); err != nil || d.Seconds() != 5 {
		t.Fatalf("5: %v %v", d, err)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERMIT_STATE_DIR", dir)
	cfg := Default()
	applyEnv(&cfg)
	if cfg.StateDir != dir {
		t.Fatalf("state dir not overridden: %s", cfg.StateDir)
	}
}

func TestLoadMissingIsOk(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err == nil {
		t.Fatal("expected error for explicit missing file")
	}
	_ = cfg
	_ = os.Getenv("HOME")
}
