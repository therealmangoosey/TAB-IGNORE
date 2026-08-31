//go:build linux || darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/internal/app"
	"github.com/therealmangoosey/TAB-IGNORE/internal/config"
)

func daemonStart(cfg config.Config) error {
	if app.ISRunning(cfg.Server.Socket) {
		return fmt.Errorf("daemon already running")
	}
	if err := app.ClearStaleSocket(cfg.Server.Socket); err != nil {
		return fmt.Errorf("clear stale daemon socket: %w", err)
	}
	if err := os.MkdirAll(cfg.LogDir, 0o755); err != nil {
		return fmt.Errorf("create daemon log directory: %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	logPath := filepath.Join(cfg.LogDir, "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open daemon log: %w", err)
	}

	cmd := exec.Command(exe, "daemon", "run")
	if v := os.Getenv("HERMIT_CONFIG"); v != "" {
		cmd.Env = append(os.Environ(), "HERMIT_CONFIG="+v)
	} else {
		cmd.Env = os.Environ()
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start daemon: %w", err)
	}
	_ = logFile.Close()

	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	// Do not report success until the child has created its RPC socket.
	for i := 0; i < 50; i++ {
		if app.ISRunning(cfg.Server.Socket) {
			fmt.Printf("daemon started (log: %s)\n", logPath)
			return nil
		}
		select {
		case err := <-exited:
			if err != nil {
				return fmt.Errorf("daemon exited during startup: %w; see %s", err, logPath)
			}
			return fmt.Errorf("daemon exited during startup; see %s", logPath)
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not become ready; see %s", logPath)
}
