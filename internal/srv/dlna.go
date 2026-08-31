package srv

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"time"

	dms "github.com/anacrolix/dms/dlna/dms"
	"github.com/therealmangoosey/TAB-IGNORE/internal/lib"
)

const dlnaHTTPAddrDefault = "0.0.0.0:8789"

type dlnaServer struct {
	library *lib.Library
	name    string
	addr    string
	server  *dms.Server
}

func newDLNAServer(library *lib.Library) *dlnaServer {
	addr := os.Getenv("HERMIT_MEDIA_SERVER_ADDR")
	if addr == "" {
		addr = dlnaHTTPAddrDefault
	}
	name := os.Getenv("HERMIT_MEDIA_SERVER_NAME")
	if name == "" {
		name = "Hermit"
	}
	return &dlnaServer{library: library, name: name, addr: addr}
}

// start launches a mature UPnP/DLNA MediaServer implementation instead of
// maintaining a partial protocol implementation in Hermit itself. The server
// exposes the library as a browseable filesystem, serves HTTP Range requests,
// and can use ffprobe/ffmpeg when available for compatibility conversions.
func (d *dlnaServer) start(ctx context.Context, logf func(string)) {
	ln, err := net.Listen("tcp", d.addr)
	if err != nil {
		if logf != nil {
			logf("DLNA listen failed: %v", err)
		}
		return
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &dms.Server{
		HTTPConn:            ln,
		FriendlyName:        d.name,
		RootObjectPath:      d.library.Root,
		NoProbe:             !commandExists("ffprobe"),
		NoTranscode:         !commandExists("ffmpeg"),
		StallEventSubscribe: true,
		IgnoreHidden:        true,
		IgnoreUnreadable:    true,
		NotifyInterval:      30 * time.Second,
		Logger:              logger,
	}
	if err := s.Init(); err != nil {
		_ = ln.Close()
		if logf != nil {
			logf("DLNA init failed: %v", err)
		}
		return
	}
	d.server = s
	if logf != nil {
		logf("DLNA media server listening on %s as %q", ln.Addr().String(), d.name)
	}

	done := make(chan error, 1)
	go func() { done <- s.Run() }()
	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()
	go func() {
		if err := <-done; err != nil && ctx.Err() == nil && logf != nil {
			logf("DLNA server stopped: %v", err)
		}
	}()
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
