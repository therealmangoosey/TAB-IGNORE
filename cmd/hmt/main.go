package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "doctor" {
		os.Exit(doctor(context.Background()))
	}
	fmt.Println("hermit bootstrap ready; use `hmt doctor` for runtime information")
}
