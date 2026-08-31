package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/internal/app"
	"github.com/therealmangoosey/TAB-IGNORE/internal/config"
	"github.com/therealmangoosey/TAB-IGNORE/internal/doctor"
	"github.com/therealmangoosey/TAB-IGNORE/internal/lib"
	"github.com/therealmangoosey/TAB-IGNORE/internal/play"
	"github.com/therealmangoosey/TAB-IGNORE/internal/provider"
	"github.com/therealmangoosey/TAB-IGNORE/internal/rpc"
	"github.com/therealmangoosey/TAB-IGNORE/internal/tui"
	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

func runCommand(ctx context.Context, cfg config.Config, cmd string, args []string) error {
	switch cmd {
	case "status":
		return cmdStatus(ctx, cfg, args)
	case "info":
		return cmdInfo(ctx, cfg, args)
	case "search":
		return cmdSearch(ctx, cfg, args)
	case "add":
		return cmdAdd(ctx, cfg, args)
	case "ls":
		return cmdList(ctx, cfg, args)
	case "get":
		return cmdGet(ctx, cfg, args)
	case "play":
		return cmdPlay(ctx, cfg, args)
	case "lib":
		return cmdLib(ctx, cfg, args)
	case "df":
		return cmdDF(ctx, cfg, args)
	case "sources":
		return cmdSources(ctx, cfg, args)
	case "db":
		return cmdDB(ctx, cfg, args)
	case "completion":
		return cmdCompletion(args)
	case "daemon":
		return cmdDaemon(ctx, cfg, args)
	case "help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q; run hmt help", cmd)
	}
}

func tuiRun(a *app.App) error {
	return tui.Run(a)
}

func cmdStatus(ctx context.Context, cfg config.Config, args []string) error {
	opts := splitArgs(args)
	a, err := newApp(cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	st, err := a.Status(ctx)
	if err != nil {
		return err
	}
	if opts["json"] == "true" || opts["json"] == "" && contains(args, "--json") {
		return printJSON(st)
	}
	fmt.Print(doctor.FormatText(st))
	return nil
}

func cmdInfo(ctx context.Context, cfg config.Config, args []string) error {
	opts := splitArgs(args)
	a, err := newApp(cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	st, err := a.Status(ctx)
	if err != nil {
		return err
	}
	if opts["json"] == "true" || contains(args, "--json") {
		return printJSON(st)
	}
	fmt.Printf("hermit %s\n", version())
	fmt.Printf("OS: %s/%s\n", st.Profile.OS, st.Profile.Arch)
	fmt.Printf("profile: %s\n", st.Profile.ProfileName)
	fmt.Printf("config: %s\n", cfg.StateDir)
	fmt.Printf("library: %s\n", cfg.Library.Path)
	fmt.Printf("server: %s\n", cfg.Server.Addr)
	return nil
}

func cmdSearch(ctx context.Context, cfg config.Config, args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "--") {
		return fmt.Errorf("usage: hmt search <query>")
	}
	query := args[0]
	kind := hermit.KindTV
	opts := splitArgs(args)
	if k := opts["kind"]; k != "" {
		kind = hermit.Kind(k)
	}
	a, err := newApp(cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	hits, err := a.Search(ctx, query, kind)
	if err != nil {
		return err
	}
	if contains(args, "--json") {
		return printJSON(hits)
	}
	for _, h := range hits {
		fmt.Printf("%s (%d) [%s tmdb=%d] %s\n", h.Title, h.Year, h.Kind, h.Ref.TMDBID, h.Provider)
	}
	if len(hits) == 0 {
		fmt.Println("no results")
	}
	return nil
}

func cmdAdd(ctx context.Context, cfg config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hmt add <url|ref> [--provider p] [--season n] [--episode n]")
	}
	opts := splitArgs(args)
	a, err := newApp(cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	target := args[0]
	job := hermit.Job{Provider: opts["provider"], Season: atoi(opts["season"]), Episode: atoi(opts["episode"]), Quality: hermit.Quality(opts["quality"]), Codec: hermit.Codec(opts["codec"])}
	if job.Provider == "" {
		job.Provider = "genericm3u8"
	}
	job.Source.URL = target
	job.Source.Kind = hermit.TransportDirect
	if strings.HasSuffix(strings.ToLower(target), ".m3u8") {
		job.Source.Kind = hermit.TransportHLS
	}
	id, err := a.Add(ctx, job)
	if err != nil {
		return err
	}
	if contains(args, "--json") {
		return printJSON(map[string]any{"id": id})
	}
	fmt.Printf("queued job %d\n", id)
	return nil
}

func cmdList(ctx context.Context, cfg config.Config, args []string) error {
	opts := splitArgs(args)
	states := strings.Split(opts["state"], ",")
	if len(states) == 1 && states[0] == "" {
		states = nil
	}
	a, err := newApp(cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	jobs, err := a.ListJobs(ctx, states)
	if err != nil {
		return err
	}
	if contains(args, "--json") {
		return printJSON(jobs)
	}
	if opts["watch"] == "true" && len(jobs) > 0 {
		for {
			jobs, err := a.ListJobs(ctx, states)
			if err != nil {
				return err
			}
			clearScreen()
			for _, j := range jobs {
				fmt.Printf("%d %s %s %d/%d\n", j.ID, j.State, j.Provider, j.BytesDone, j.BytesTotal)
			}
			time.Sleep(2 * time.Second)
			if ctx.Err() != nil {
				return nil
			}
		}
	}
	for _, j := range jobs {
		fmt.Printf("%d\t%s\t%s\t%s\t%d/%d\t%s\n", j.ID, j.State, j.Provider, j.Quality, j.BytesDone, j.BytesTotal, j.LastError)
	}
	return nil
}

func cmdGet(ctx context.Context, cfg config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hmt get <url> [--out path]")
	}
	opts := splitArgs(args)
	a, err := newApp(cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	reg, err := provider.NewRegistry(cfg)
	if err != nil {
		return err
	}
	srcs, err := reg.Resolve(ctx, "genericm3u8", hermit.Ref{Title: args[0]})
	if err != nil {
		return err
	}
	if len(srcs) == 0 {
		return fmt.Errorf("empty source")
	}
	out := opts["out"]
	if out == "" {
		out = filepath.Join(cfg.Library.Path, "Downloads", filepath.Base(args[0]))
	}
	res, err := a.Fetcher.Download(ctx, srcs[0], out)
	if err != nil {
		return err
	}
	if contains(args, "--json") {
		return printJSON(res)
	}
	fmt.Printf("downloaded %s (%d bytes, sha256 %s)\n", res.Path, res.Bytes, res.SHA256)
	return nil
}

func cmdPlay(ctx context.Context, cfg config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hmt play <id|path|url> [--resume]")
	}
	opts := splitArgs(args)
	a, err := newApp(cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	target := args[0]
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		if err := openURL(ctx, target); err != nil {
			return err
		}
		fmt.Println(target)
		return nil
	}
	if filepath.Ext(target) != "" {
		rel, _ := filepath.Rel(cfg.Library.Path, target)
		if strings.HasPrefix(rel, "..") {
			rel = target
		}
		mediaURL := serverURL(cfg) + "/media/" + filepath.ToSlash(rel)
		if err := openURL(ctx, mediaURL); err != nil {
			return err
		}
		fmt.Println(mediaURL)
		return nil
	}
	id, _ := strconv.ParseInt(target, 10, 64)
	season := atoi(opts["season"])
	episode := atoi(opts["episode"])
	mediaURL, err := a.Play(ctx, id, season, episode, opts["resume"] == "true")
	if err != nil {
		return err
	}
	fmt.Println(mediaURL)
	return nil
}

func cmdLib(ctx context.Context, cfg config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hmt lib <list|scan|gc|secure|reclaim>")
	}
	opts := splitArgs(args)
	lf := lib.New(cfg.Library.Path, cfg.Disk.Reserve, cfg.Disk.Margin)
	switch args[0] {
	case "list", "scan":
		entries, err := lf.Scan(ctx)
		if err != nil {
			return err
		}
		if contains(args, "--json") {
			return printJSON(entries)
		}
		for _, e := range entries {
			fmt.Println(e.Path)
		}
		return nil
	case "secure":
		n, err := lf.Secure()
		if err != nil {
			return err
		}
		fmt.Printf("wrote .nomedia to %d show directories\n", n)
		return nil
	case "reclaim":
		keep, _ := config.ParseSize(opts["keep-fitting"])
		if keep == 0 {
			keep = 1 << 30
		}
		n, bytes, err := lf.Reclaim(ctx, func(string) bool { return true }, keep)
		if err != nil {
			return err
		}
		fmt.Printf("reclaimed %d files (%d bytes)\n", n, bytes)
		return nil
	case "gc":
		return cmdLibGC(ctx, cfg, lf)
	default:
		return fmt.Errorf("unknown lib command %q", args[0])
	}
}

func cmdLibGC(ctx context.Context, cfg config.Config, lf *lib.Library) error {
	entries, err := lf.Scan(ctx)
	if err != nil {
		return err
	}
	var total int64
	for _, e := range entries {
		total += e.Size
	}
	fmt.Printf("library contains %d files, %.1f GB\n", len(entries), float64(total)/1e9)
	return nil
}

func cmdDF(ctx context.Context, cfg config.Config, args []string) error {
	a, err := newApp(cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	st, err := a.Status(ctx)
	if err != nil {
		return err
	}
	if contains(args, "--json") {
		return printJSON(map[string]any{"free": st.FreeBytes, "reserve": st.ReserveBytes, "spare": st.SpareBytes})
	}
	fmt.Printf("free=%.1fGB reserve=%.1fGB spare=%.1fGB\n", float64(st.FreeBytes)/1e9, float64(st.ReserveBytes)/1e9, float64(st.SpareBytes)/1e9)
	return nil
}

func cmdSources(ctx context.Context, cfg config.Config, args []string) error {
	a, err := newApp(cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	st, err := a.Status(ctx)
	if err != nil {
		return err
	}
	if contains(args, "--json") {
		return printJSON(st.Providers)
	}
	for _, p := range st.Providers {
		mark := "✓"
		if !p.OK {
			mark = "✗"
		}
		fmt.Printf("%s %s %s\n", mark, p.ID, p.Base)
	}
	return nil
}

func cmdDB(ctx context.Context, cfg config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hmt db vacuum")
	}
	a, err := newApp(cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	if args[0] == "vacuum" {
		if err := a.DB.Vacuum(ctx); err != nil {
			return err
		}
		fmt.Println("vacuum complete")
		return nil
	}
	return fmt.Errorf("unknown db command %q", args[0])
}

func cmdDaemon(ctx context.Context, cfg config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hmt daemon start|stop|status|run")
	}
	switch args[0] {
	case "start":
		return daemonStart(cfg)
	case "run":
		return daemonRun(ctx, cfg)
	case "status":
		if app.ISRunning(cfg.Server.Socket) {
			fmt.Println("running")
			return nil
		}
		fmt.Println("stopped")
		return nil
	case "stop":
		client, err := rpc.Dial(cfg.Server.Socket)
		if err != nil {
			return err
		}
		defer client.Close()
		var empty rpc.Empty
		return client.Call("hermit.Shutdown", &empty, &empty)
	default:
		return fmt.Errorf("unknown daemon command %q", args[0])
	}
}

func daemonRun(ctx context.Context, cfg config.Config) error {
	a, err := newApp(cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	ln, _, err := rpc.Serve(cfg.Server.Socket, a)
	if err != nil {
		return err
	}
	defer ln.Close()
	a.StartDaemon(ctx)
	// Keep running until Shutdown is called over RPC.
	<-a.Done()
	return nil
}

func cmdCompletion(args []string) error {
	shell := ""
	if len(args) > 0 {
		shell = args[0]
	}
	switch shell {
	case "bash":
		fmt.Println(`complete -W "status info doctor search add ls get play lib df sources db daemon completion version" hmt`)
	case "zsh":
		fmt.Println(`compdef _hmt hmt`)
	case "fish":
		fmt.Println(`complete -c hmt -f`)
	default:
		return fmt.Errorf("supported shells: bash, zsh, fish")
	}
	return nil
}

func openURL(ctx context.Context, target string) error {
	return play.Open(ctx, target, "video/*")
}

func serverURL(cfg config.Config) string {
	addr := cfg.Server.Addr
	if strings.HasPrefix(addr, "127.0.0.1:") {
		return "http://127.0.0.1:" + strings.TrimPrefix(addr, "127.0.0.1:")
	}
	return "http://" + addr
}

func clearScreen() {
	fmt.Print("\033[2J\033[H")
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func contains(args []string, s string) bool {
	for _, a := range args {
		if a == s {
			return true
		}
	}
	return false
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}


