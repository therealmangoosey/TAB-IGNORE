package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/therealmangoosey/TAB-IGNORE/internal/config"
	doctorpkg "github.com/therealmangoosey/TAB-IGNORE/internal/doctor"
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

	formatted := doctorpkg.FormatText(st)
	if strings.TrimSpace(formatted) == "" {
		formatted = "hermit: no diagnostics available\n"
	}
	fmt.Print(formatted)

	// Keep doctor useful even when the database cannot be opened.
	if cfg.StateDir != "" {
		fmt.Printf("state: %s\n", cfg.StateDir)
	}
	if cfg.Library.Path != "" {
		fmt.Printf("library: %s\n", cfg.Library.Path)
	}
	return 0
}
