// Package config loads the hermit configuration from TOML, environment
// variables, and command-line overrides.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// ProviderConfig is one entry in [[providers.entries]].
type ProviderConfig struct {
	ID          string   `toml:"id"`
	Enabled     bool     `toml:"enabled"`
	Base        string   `toml:"base"`
	EmbedHosts  []string `toml:"embed_hosts"`
	Fallback    []string `toml:"fallback_chain"`
	AllowUnofficial bool `toml:"allow_unofficial"`
	IDSpace     string   `toml:"id_space"`
	Token       string   `toml:"token"`
	Headers     map[string]string `toml:"headers"`
}

// PowerConfig controls battery-aware scheduling.
type PowerConfig struct {
	WakeLock        string `toml:"wake_lock"`
	MinBatteryPct   int    `toml:"min_battery_pct"`
	DeferRemuxToCharge bool `toml:"defer_remux_to_charge"`
	ConcurrencyBattery int `toml:"concurrency_battery"`
	ConcurrencyCharging int `toml:"concurrency_charging"`
	MaxBytesPerSec  string `toml:"max_bytes_per_sec"`
	NiceRemux       int    `toml:"nice_remux"`
	PauseWhenThermal string `toml:"pause_when_thermal"`
}

// DiskConfig controls storage headroom.
type DiskConfig struct {
	Reserve        string `toml:"reserve"`
	ReserveInternal string `toml:"reserve_internal"`
	Margin         string `toml:"margin"`
	Staging        string `toml:"staging"`
}

// LibraryConfig controls the library.
type LibraryConfig struct {
	Path       string `toml:"path"`
	Art        bool   `toml:"art"`
	NoMedia    bool   `toml:"nomedia"`
	Extension  string `toml:"extension"`
}

// MetaConfig controls metadata providers.
type MetaConfig struct {
	TMDBKey      string `toml:"tmdb_key"`
	AniListKey   string `toml:"anilist_key"`
	CacheTTL     string `toml:"cache_ttl"`
	Language     string `toml:"language"`
}

// ServerConfig controls the local HTTP server and RPC socket.
type ServerConfig struct {
	Addr   string `toml:"addr"`
	Lan    string `toml:"lan"`
	Socket string `toml:"socket"`
}

// TUIConfig contains TUI display preferences.
type TUIConfig struct {
	FontScale int    `toml:"font_scale"`
	Theme     string `toml:"theme"`
}

// Config is the complete merged configuration.
type Config struct {
	Home       string                 `toml:"home"`
	StateDir   string                 `toml:"state_dir"`
	LogDir     string                 `toml:"log_dir"`
	CacheDir   string                 `toml:"cache_dir"`
	Providers  ProvidersConfig        `toml:"providers"`
	Power      PowerConfig            `toml:"power"`
	Disk       DiskConfig             `toml:"disk"`
	Library    LibraryConfig          `toml:"library"`
	Meta       MetaConfig             `toml:"meta"`
	Server     ServerConfig           `toml:"server"`
	TUI        TUIConfig              `toml:"tui"`
	Extra      map[string]any         `toml:"extra"`
}

// ProvidersConfig is the top-level providers section.
type ProvidersConfig struct {
	ExtraPaths []string         `toml:"extra_paths"`
	PersistCookies bool         `toml:"persist_cookies"`
	Entries    []ProviderConfig `toml:"entries"`
}

// Default returns a conservative default configuration for a low-resource
// Android/Termux device.
func Default() Config {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "."
	}
	state := filepath.Join(home, ".hermit")
	return Config{
		Home:     home,
		StateDir: state,
		LogDir:   filepath.Join(state, "logs"),
		CacheDir: filepath.Join(state, "cache"),
		Providers: ProvidersConfig{
			PersistCookies: false,
			Entries: []ProviderConfig{
				{ID: "localfs", Enabled: true, Base: ""},
				{ID: "archiveorg", Enabled: true, Base: "https://archive.org"},
				{ID: "genericm3u8", Enabled: true, Base: "", AllowUnofficial: false},
			},
		},
		Power: PowerConfig{
			WakeLock:            "auto",
			MinBatteryPct:       12,
			DeferRemuxToCharge:  true,
			ConcurrencyBattery:  4,
			ConcurrencyCharging: 8,
			MaxBytesPerSec:      "4MiB",
			NiceRemux:           10,
			PauseWhenThermal:    "moderate",
		},
		Disk: DiskConfig{
			Reserve:         "3GB",
			ReserveInternal: "512MB",
			Margin:          "64MB",
			Staging:         "auto",
		},
		Library: LibraryConfig{
			Path:      filepath.Join(home, "Media", "hermit"),
			Art:       false,
			NoMedia:   true,
			Extension: "mp4",
		},
		Meta: MetaConfig{
			CacheTTL: "24h",
			Language: "en-US",
		},
		Server: ServerConfig{
			Addr:   "127.0.0.1:8788",
			Socket: filepath.Join(state, "hmt.sock"),
		},
		TUI: TUIConfig{
			FontScale: 1,
			Theme:     "dark",
		},
	}
}

// Load reads config from the explicit path, or the default locations.
// Env vars override TOML values, with HERMIT_ as the prefix.
// The returned Config is always a fully populated struct.
func Load(path string) (Config, error) {
	cfg := Default()

	choose := path
	if choose == "" {
		choose = os.Getenv("HERMIT_CONFIG")
	}
	if choose == "" {
		choose = filepath.Join(cfg.StateDir, "config.toml")
	}

	if _, err := os.Stat(choose); err == nil {
		if _, err := toml.DecodeFile(choose, &cfg); err != nil {
			return cfg, fmt.Errorf("load config %s: %w", choose, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return cfg, fmt.Errorf("stat config %s: %w", choose, err)
	} else if path != "" {
		return cfg, fmt.Errorf("config file %s not found", path)
	}

	applyEnv(&cfg)
	applyDefaults(&cfg)
	return cfg, nil
}

func applyEnv(cfg *Config) {
	setStr := func(key string, dst *string) {
		if v := os.Getenv(key); v != "" {
			*dst = v
		}
	}
	setStr("HERMIT_STATE_DIR", &cfg.StateDir)
	setStr("HERMIT_LIBRARY_PATH", &cfg.Library.Path)
	setStr("HERMIT_SERVER_ADDR", &cfg.Server.Addr)
	setStr("HERMIT_RPC_SOCKET", &cfg.Server.Socket)
	setStr("HERMIT_TMDB_KEY", &cfg.Meta.TMDBKey)
	setStr("HERMIT_LOG_DIR", &cfg.LogDir)
	setStr("HERMIT_CACHE_DIR", &cfg.CacheDir)
}

func applyDefaults(cfg *Config) {
	if cfg.StateDir == "" {
		cfg.StateDir = filepath.Join(cfg.Home, ".hermit")
	}
	if cfg.LogDir == "" {
		cfg.LogDir = filepath.Join(cfg.StateDir, "logs")
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = filepath.Join(cfg.StateDir, "cache")
	}
	if cfg.Server.Socket == "" {
		cfg.Server.Socket = filepath.Join(cfg.StateDir, "hmt.sock")
	}
	if cfg.Library.Path == "" {
		cfg.Library.Path = filepath.Join(cfg.Home, "Media", "hermit")
	}
}

// ParseSize parses a size string like "4MiB", "3GB", "512MB", or a bare byte
// count. Units are binary (MiB, GiB) when present, decimal otherwise.
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty size")
	}
	mult := int64(1)
	upper := strings.ToUpper(s)
	suffixes := []struct {
		suffix string
		mult   int64
	}{
		{"TIB", 1 << 40}, {"TB", 1_000_000_000_000},
		{"GIB", 1 << 30}, {"GB", 1_000_000_000},
		{"MIB", 1 << 20}, {"MB", 1_000_000},
		{"KIB", 1 << 10}, {"KB", 1_000},
		{"B", 1},
	}
	for _, sfx := range suffixes {
		if strings.HasSuffix(upper, sfx.suffix) {
			mult = sfx.mult
			s = strings.TrimSpace(s[:len(s)-len(sfx.suffix)])
			break
		}
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse size %q: %w", s, err)
	}
	return n * mult, nil
}

// ParseDuration parses a duration like "24h" or "5m". It accepts plain
// integers as seconds.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty duration")
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", s, err)
	}
	return time.Duration(n) * time.Second, nil
}
