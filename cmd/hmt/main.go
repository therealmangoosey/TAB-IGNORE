// Command hmt is the hermit CLI and TUI.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/therealmangoosey/TAB-IGNORE/internal/app"
	"github.com/therealmangoosey/TAB-IGNORE/internal/config"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load(os.Getenv("HERMIT_CONFIG"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(2)
	}
	if len(os.Args) < 2 {
		if isTTY() {
			if err := runTUI(ctx, cfg); err != nil {
				fmt.Fprintln(os.Stderr, "tui:", err)
				os.Exit(1)
			}
			return
		}
		usage()
		return
	}
	if os.Args[1] == "doctor" {
		os.Exit(runDoctor(ctx, cfg, os.Args[2:]))
	}
	if os.Args[1] == "version" {
		fmt.Println("hermit", version())
		return
	}
	if os.Args[1] == "vpn" {
		if err := cmdVPN(ctx, cfg, os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "hmt vpn:", err)
			os.Exit(1)
		}
		return
	}
	if err := runCommand(ctx, cfg, os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "hmt:", err)
		os.Exit(1)
	}
}

func isTTY() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func usage() {
	fmt.Fprintf(os.Stderr, `hermit %s

Usage:
  hmt                     open the TUI
  hmt status              daemon and library status
  hmt info                runtime information
  hmt doctor              hardware/runtime diagnostics
  hmt daemon start|stop|status|run
  hmt vpn up <conf>       enable split-tunnel WireGuard for hmt traffic
  hmt vpn start <conf>    enable split tunnel and run the daemon
  hmt vpn down            disable the split tunnel
  hmt vpn status          show split-tunnel status
  hmt search <query>      search metadata and providers
  hmt add <url|ref>       queue a download
  hmt ls                  list download jobs
  hmt get <url>           download a direct/HLS URL
  hmt play <id|path|url>  stream/hand off to a player
  hmt lib list|scan|gc|secure|reclaim
  hmt df                  disk headroom
  hmt sources             provider health
  hmt db vacuum           maintain the database
  hmt completion <shell>  shell completions
  hmt version
`, version())
}

func newApp(cfg config.Config) (*app.App, error) {
	if err := app.EnsureStorage(cfg); err != nil {
		return nil, err
	}
	return app.New(cfg)
}

func runTUI(ctx context.Context, cfg config.Config) error {
	a, err := newApp(cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	return tuiRun(a)
}

func version() string {
	return "0.1.0"
}

func splitArgs(args []string) map[string]string {
	out := map[string]string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			key := strings.TrimPrefix(arg, "--")
			if eq := strings.IndexByte(key, '='); eq >= 0 {
				out[key[:eq]] = key[eq+1:]
			} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				out[key] = args[i+1]
				i++
			} else {
				out[key] = "true"
			}
		}
	}
	return out
}
