// Package app wires the config, database, providers, queue, HTTP server, and
// RPC service into one daemon instance used by the CLI and TUI.
package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/internal/config"
	"github.com/therealmangoosey/TAB-IGNORE/internal/db"
	"github.com/therealmangoosey/TAB-IGNORE/internal/doctor"
	"github.com/therealmangoosey/TAB-IGNORE/internal/fetch"
	"github.com/therealmangoosey/TAB-IGNORE/internal/lib"
	"github.com/therealmangoosey/TAB-IGNORE/internal/meta"
	"github.com/therealmangoosey/TAB-IGNORE/internal/play"
	"github.com/therealmangoosey/TAB-IGNORE/internal/provider"
	"github.com/therealmangoosey/TAB-IGNORE/internal/queue"
	"github.com/therealmangoosey/TAB-IGNORE/internal/scrub"
	"github.com/therealmangoosey/TAB-IGNORE/internal/srv"
	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

// App is the fully wired application.
type App struct {
	Cfg      config.Config
	DB       *db.DB
	Registry *provider.Registry
	Meta     *meta.Client
	Library  *lib.Library
	Queue    *queue.Queue
	Server   *srv.Server
	Fetcher  *fetch.Downloader

	mu        sync.Mutex
	closeOnce sync.Once
	started   time.Time
	stopping  bool
	stop      context.CancelFunc
	done      chan struct{}
}

// New constructs an app.
func New(cfg config.Config) (*App, error) {
	database, err := db.Open(filepath.Join(cfg.StateDir, "hermit.db"))
	if err != nil {
		return nil, err
	}
	reg, err := provider.NewRegistry(cfg)
	if err != nil {
		database.Close()
		return nil, err
	}
	library := lib.New(cfg.Library.Path, cfg.Disk.Reserve, cfg.Disk.Margin)
	maxRate, _ := config.ParseSize(cfg.Power.MaxBytesPerSec)
	dl := fetch.NewDownloader(fetch.NewClient(nil), maxRate, cfg.Power.ConcurrencyBattery)
	q := queue.NewQueue(database, reg, dl, cfg)
	cacheTTL, err := config.ParseDuration(cfg.Meta.CacheTTL)
	if err != nil {
		cacheTTL = 24 * time.Hour
	}
	metaClient := meta.NewClientWithTTL(database, fetch.NewClient([]string{"https://api.themoviedb.org"}), cfg.Meta.TMDBKey, cacheTTL)
	server := srv.New(cfg.Server.Addr, cfg.Server.Lan, library, func(ctx context.Context) (hermit.Status, error) {
		return Status(ctx, cfg, database, reg, library)
	})
	return &App{Cfg: cfg, DB: database, Registry: reg, Meta: metaClient, Library: library, Queue: q, Server: server, Fetcher: dl, started: time.Now(), done: make(chan struct{})}, nil
}

// Close closes resources.
func (a *App) Close() error {
	return a.DB.Close()
}

// StartDaemon starts the queue pump and HTTP server in the background.
func (a *App) StartDaemon(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.stop = cancel
	a.mu.Unlock()
	go a.Queue.Pump(ctx)
	go a.Server.Start(ctx)
}

// Stop cancels the running daemon.
func (a *App) Stop() {
	a.mu.Lock()
	if a.stop != nil {
		a.stop()
	}
	a.mu.Unlock()
	a.closeOnce.Do(func() { close(a.done) })
}

// Done returns a channel that is closed when Stop is called.
func (a *App) Done() <-chan struct{} {
	return a.done
}

// Status implements the RPC/status contract.
func Status(ctx context.Context, cfg config.Config, database *db.DB, reg *provider.Registry, library *lib.Library) (hermit.Status, error) {
	return doctor.Probe(ctx, cfg, reg, filepath.Join(cfg.StateDir, "hermit.db"), func(st *hermit.Status) {
		jobList, _ := database.ListJobs(ctx, nil, 200)
		st.Jobs = jobList
		for _, name := range reg.Names() {
			hp := hermit.ProviderHealth{ID: name, Enabled: true, Base: reg.Base(name), OK: true}
			st.Providers = append(st.Providers, hp)
		}
		if n, size, err := library.Summary(ctx); err == nil {
			st.LibraryFiles = n
			st.LibrarySize = size
		}
	})
}

// Status returns the daemon status snapshot.
func (a *App) Status(ctx context.Context) (hermit.Status, error) {
	return Status(ctx, a.Cfg, a.DB, a.Registry, a.Library)
}

// Search implements rpc.Backend.
func (a *App) Search(ctx context.Context, query string, kind hermit.Kind) ([]hermit.Hit, error) {
	hits, err := a.Registry.Search(ctx, query, kind)
	if err == nil && len(hits) == 0 && a.Cfg.Meta.TMDBKey != "" {
		hits, err = a.Meta.Search(ctx, query, kind)
	}
	return hits, err
}

// Add implements rpc.Backend.
func (a *App) Add(ctx context.Context, job hermit.Job) (int64, error) {
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	if job.State == "" {
		job.State = hermit.JobQueued
	}
	if job.Priority == 0 {
		job.Priority = 100
	}
	if job.Provider == "" {
		job.Provider = "genericm3u8"
	}
	if job.Source.URL != "" {
		srcs, err := a.Registry.Resolve(ctx, job.Provider, hermit.Ref{Title: job.Source.URL})
		if err == nil && len(srcs) > 0 {
			job.Source = srcs[0]
		}
	}
	return a.DB.InsertJob(ctx, job)
}

// ListJobs implements rpc.Backend.
func (a *App) ListJobs(ctx context.Context, states []string) ([]hermit.Job, error) {
	return a.DB.ListJobs(ctx, states, 500)
}

// Play implements rpc.Backend.
func (a *App) Play(ctx context.Context, mediaID int64, season, episode int, resume bool) (string, error) {
	base := "http://127.0.0.1" + a.Cfg.Server.Addr
	if strings.HasPrefix(a.Cfg.Server.Addr, "127.0.0.1") && len(a.Cfg.Server.Addr) > 9 {
		base = "http://127.0.0.1:" + strings.TrimPrefix(a.Cfg.Server.Addr, "127.0.0.1:")
	}
	files, err := a.Library.Scan(ctx)
	if err != nil {
		return "", err
	}
	for _, f := range files {
		if matchEpisode(f.Path, mediaID, season, episode) {
			rel, _ := filepath.Rel(a.Library.Root, f.Path)
			url := base + "/media/" + filepath.ToSlash(rel)
			if resume {
				_ = resume
			}
			if err := play.Open(ctx, url, "video/*"); err != nil {
				return "", err
			}
			return url, nil
		}
	}
	return "", fmt.Errorf("no local file found for media=%d S%dE%d", mediaID, season, episode)
}

func matchEpisode(path string, mediaID int64, season, episode int) bool {
	name := filepath.Base(path)
	needle := fmt.Sprintf("S%02dE%02d", season, episode)
	if !strings.Contains(strings.ToUpper(name), needle) {
		return false
	}
	_ = mediaID
	return true
}

// Doctor implements rpc.Backend.
func (a *App) Doctor(ctx context.Context) (hermit.Status, error) {
	return Status(ctx, a.Cfg, a.DB, a.Registry, a.Library)
}

// Shutdown implements rpc.Backend.
func (a *App) Shutdown(ctx context.Context) error {
	a.Stop()
	return nil
}

// DBPath returns the database path.
func (a *App) DBPath() string {
	return filepath.Join(a.Cfg.StateDir, "hermit.db")
}

// EnsureStorage makes key directories.
func EnsureStorage(cfg config.Config) error {
	for _, dir := range []string{cfg.StateDir, cfg.LogDir, cfg.CacheDir, cfg.Library.Path} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// ISRunning reports whether a daemon socket is live.
func ISRunning(socket string) bool {
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// OpenFileForExternal opens a local file with the platform player.
func OpenFileForExternal(ctx context.Context, path string) error {
	return play.Open(ctx, "file://"+path, "video/*")
}

// CommandAvailable is used by doctor.
func CommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Platform is the runtime OS string.
func Platform() string { return runtime.GOOS }

// ScrubFile normalizes a file label; retained as a tiny wrapper for callers.
func ScrubFile(name string) string {
	return scrub.SafeName(name)
}

// ClearStaleSocket removes a dead socket.
func ClearStaleSocket(socket string) error {
	if _, err := os.Stat(socket); err != nil {
		return nil
	}
	if ISRunning(socket) {
		return errors.New("socket is already in use")
	}
	return os.Remove(socket)
}
