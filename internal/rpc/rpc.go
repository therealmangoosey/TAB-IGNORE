// Package rpc implements the daemon's Unix-socket JSON-RPC surface used by the
// TUI and scripting. It uses net/rpc + jsonrpc from the standard library so
// both sides share one code path with no extra dependency.
package rpc

import (
	"context"
	"fmt"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"path/filepath"

	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

// Backend is the subset of the daemon the RPC service exposes.
type Backend interface {
	Status(ctx context.Context) (hermit.Status, error)
	Search(ctx context.Context, query string, kind hermit.Kind) ([]hermit.Hit, error)
	Add(ctx context.Context, job hermit.Job) (int64, error)
	ListJobs(ctx context.Context, states []string) ([]hermit.Job, error)
	Play(ctx context.Context, mediaID int64, season, episode int, resume bool) (string, error)
	Doctor(ctx context.Context) (hermit.Status, error)
	Shutdown(ctx context.Context) error
}

// Service is the exported net/rpc service.
type Service struct {
	Backend Backend
}

// SearchArgs is the search request.
type SearchArgs struct {
	Query string
	Kind  hermit.Kind
}

// SearchReply is the search response.
type SearchReply struct {
	Hits []hermit.Hit
}

// AddArgs is the add request.
type AddArgs struct {
	Job hermit.Job
}

// AddReply is the add response.
type AddReply struct {
	ID int64
}

// ListJobsArgs lists jobs.
type ListJobsArgs struct {
	States []string
}

// ListJobsReply returns jobs.
type ListJobsReply struct {
	Jobs []hermit.Job
}

// PlayArgs requests a player handoff.
type PlayArgs struct {
	MediaID int64
	Season  int
	Episode int
	Resume  bool
	URL     string
}

// PlayReply contains the local stream URL.
type PlayReply struct {
	URL string
}

// Empty is a no-op request/reply.
type Empty struct{}

// Search handles search.
func (s *Service) Search(args *SearchArgs, reply *SearchReply) error {
	if s.Backend == nil {
		return fmt.Errorf("rpc backend not set")
	}
	hits, err := s.Backend.Search(context.Background(), args.Query, args.Kind)
	if err != nil {
		return err
	}
	reply.Hits = hits
	return nil
}

// Add handles adding a job.
func (s *Service) Add(args *AddArgs, reply *AddReply) error {
	if s.Backend == nil {
		return fmt.Errorf("rpc backend not set")
	}
	id, err := s.Backend.Add(context.Background(), args.Job)
	if err != nil {
		return err
	}
	reply.ID = id
	return nil
}

// ListJobs handles listing jobs.
func (s *Service) ListJobs(args *ListJobsArgs, reply *ListJobsReply) error {
	if s.Backend == nil {
		return fmt.Errorf("rpc backend not set")
	}
	jobs, err := s.Backend.ListJobs(context.Background(), args.States)
	if err != nil {
		return err
	}
	reply.Jobs = jobs
	return nil
}

// Play handles player handoff.
func (s *Service) Play(args *PlayArgs, reply *PlayReply) error {
	if s.Backend == nil {
		return fmt.Errorf("rpc backend not set")
	}
	url, err := s.Backend.Play(context.Background(), args.MediaID, args.Season, args.Episode, args.Resume)
	if err != nil {
		return err
	}
	if url == "" {
		url = args.URL
	}
	reply.URL = url
	return nil
}

// Doctor handles diagnostics.
func (s *Service) Doctor(args *Empty, reply *hermit.Status) error {
	if s.Backend == nil {
		return fmt.Errorf("rpc backend not set")
	}
	st, err := s.Backend.Doctor(context.Background())
	if err != nil {
		return err
	}
	*reply = st
	return nil
}

// Shutdown stops the daemon.
func (s *Service) Shutdown(args *Empty, reply *Empty) error {
	if s.Backend == nil {
		return fmt.Errorf("rpc backend not set")
	}
	return s.Backend.Shutdown(context.Background())
}

// Serve starts a JSON-RPC server on a Unix socket.
func Serve(socket string, backend Backend) (*net.UnixListener, *rpc.Server, error) {
	if err := os.MkdirAll(filepath.Dir(socket), 0o755); err != nil {
		return nil, nil, err
	}
	if info, err := os.Stat(socket); err == nil && info.Mode()&os.ModeSocket != 0 {
		if err := os.Remove(socket); err != nil {
			return nil, nil, fmt.Errorf("remove stale socket: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	addr, err := net.ResolveUnixAddr("unix", socket)
	if err != nil {
		return nil, nil, err
	}
	ln, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, nil, err
	}
	_ = os.Chmod(socket, 0o600)

	server := rpc.NewServer()
	if err := server.RegisterName("hermit", &Service{Backend: backend}); err != nil {
		_ = ln.Close()
		_ = os.Remove(socket)
		return nil, nil, err
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go server.ServeCodec(jsonrpc.NewServerCodec(conn))
		}
	}()
	return ln, server, nil
}

// Dial connects to a running daemon socket.
func Dial(socket string) (*rpc.Client, error) {
	if info, err := os.Stat(socket); err != nil || info.Mode()&os.ModeSocket == 0 {
		return nil, fmt.Errorf("daemon socket %s is not running; start it with hmt daemon start", socket)
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, err
	}
	return jsonrpc.NewClient(conn), nil
}
