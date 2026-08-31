package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/therealmangoosey/TAB-IGNORE/internal/app"
	"github.com/therealmangoosey/TAB-IGNORE/internal/config"
	"github.com/therealmangoosey/TAB-IGNORE/internal/doctor"
)

func doctor(ctx context.Context, cfg config.Config, args []string) int {
	a, err := newApp(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "doctor:", err)
		return 1
	}
	defer a.Close()
	st, err := a.Doctor(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "doctor:", err)
		return 1
	}
	if contains(args, "--json") {
		if err := printJSON(st); err != nil {
			fmt.Fprintln(os.Stderr, "doctor:", err)
			return 1
		}
		return 0
	}
	// Probe URLs for health.
	for i := range st.Providers {
		hp := &st.Providers[i]
		if hp.Base == "" {
			hp.OK = true
			continue
		}
		fmt.Printf("%s: %s\n", hp.ID, hp.Base)
	}
	fmt.Print(doctor.FormatText(st))
	if strings.TrimSpace(doctor.FormatText(st)) == "" {
		fmt.Println("doctor: no diagnostics available")
	}
	return 0
}
