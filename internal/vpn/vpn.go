package vpn

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const DefaultMark = 42420

func StatePath() string {
	if state := os.Getenv("HERMIT_STATE_DIR"); state != "" {
		return filepath.Join(state, "vpn-state")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hermit", "vpn-state")
}

func ActiveMark() int {
	b, err := os.ReadFile(StatePath())
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func writeState(mark int) error {
	if mark <= 0 {
		return errors.New("invalid VPN mark")
	}
	if err := os.MkdirAll(filepath.Dir(StatePath()), 0o700); err != nil {
		return err
	}
	return os.WriteFile(StatePath(), []byte(strconv.Itoa(mark)+"\n"), 0o600)
}

func clearState() error {
	if err := os.Remove(StatePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
