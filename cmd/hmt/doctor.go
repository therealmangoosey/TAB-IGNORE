package main

import (
	"context"
	"fmt"
	"runtime"
)

func doctor(ctx context.Context) int {
	select {
	case <-ctx.Done():
		return 1
	default:
	}
	fmt.Printf("hermit doctor (bootstrap)\nOS: %s\nArch: %s\nGo: %s\n", runtime.GOOS, runtime.GOARCH, runtime.Version())
	return 0
}
