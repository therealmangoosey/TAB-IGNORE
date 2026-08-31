//go:build linux || darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/therealmangoosey/TAB-IGNORE/internal/app"
	"github.com/therealmangoosey/TAB-IGNORE/internal/config"
)

func daemonStart(cfg config.Config) error {
	if app.ISRunning(cfg.Server.Socket) {
		return fmt.Errorf("daemon already running")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "daemon", "run")
	cmd.Env = append(os.Environ(), "HERMIT_CONFIG="+os.Getenv("HERMIT_CONFIG"))
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	fmt.Println("daemon started")
	return nil
}
