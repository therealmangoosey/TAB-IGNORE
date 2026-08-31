package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/therealmangoosey/TAB-IGNORE/internal/config"
	doctorpkg "github.com/therealmangoosey/TAB-IGNORE/internal/doctor"
	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

// runDoctor intentionally does not construct the full application. Doctor is
// a lightweight diagnostic command and must still work when the SQLite layer
// or another optional runtime dependency is unavailable.
func runDoctor(ctx context.Context, cfg config.Config, args []string) int {
	st, err := doctorpkg.Probe(ctx, cfg, nil, filepath.Join(cfg.StateDir, "hermit.db"), nil)
	if err != nil {
		fmt.Println("doctor:", err)
		return 1
	}

	if contains(args, "--json") {
		if err := printJSON(st); err != nil {
			fmt.Println("doctor:", err)
			return 1
		}
		return 0
	}

	// Provider configuration is intentionally omitted here. A doctor run must
	// not require opening provider clients or the database just to report the
	// local device state.
	formatted := doctorpkg.FormatText(st)
	if strings.TrimSpace(formatted) == "" {
		formatted = "hermit: no diagnostics available\n"
	}
	fmt.Print(formatted)

	// Keep the command honest about paths without opening SQLite.
	if cfg.StateDir != "" {
		fmt.Printf("state: %s\n", cfg.StateDir)
	}
	if cfg.Library.Path != "" {
		fmt.Printf("library: %s\n", cfg.Library.Path)
	}
	_ = hermit.Status{}
	return 0
}
