package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/therealmangoosey/TAB-IGNORE/internal/config"
	doctorpkg "github.com/therealmangoosey/TAB-IGNORE/internal/doctor"
)

func runDoctor(ctx context.Context, cfg config.Config, args []string) int {
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
	for i := range st.Providers {
		hp := &st.Providers[i]
		if hp.Base == "" {
			hp.OK = true
			continue
		}
		fmt.Printf("%s: %s\n", hp.ID, hp.Base)
	}
	formatted := doctorpkg.FormatText(st)
	fmt.Print(formatted)
	if strings.TrimSpace(formatted) == "" {
		fmt.Println("doctor: no diagnostics available")
	}
	return 0
}
