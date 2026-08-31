//go:build linux

package vpn

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	interfaceName = "hmtvpn"
	routeTable    = "42420"
)

func command(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func ensureRoot() error {
	if os.Geteuid() != 0 {
		return errors.New("split-tunnel WireGuard setup requires root; other device traffic is left untouched")
	}
	return nil
}

func Up(configPath string) error {
	if err := ensureRoot(); err != nil {
		return err
	}
	if configPath == "" {
		return errors.New("usage: hmt vpn up <wireguard.conf>")
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read WireGuard config: %w", err)
	}
	stateDir := filepath.Dir(StatePath())
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	generated := filepath.Join(stateDir, interfaceName+".conf")
	text := string(data)
	if !strings.Contains(text, "\nTable =") && !strings.Contains(text, "\nTable=") {
		text = addInterfaceSetting(text, "Table = off")
	}
	if err := os.WriteFile(generated, []byte(text), 0o600); err != nil {
		return err
	}
	_ = Down()
	if err := command("wg-quick", "up", generated); err != nil {
		return err
	}
	if err := command("ip", "rule", "add", "fwmark", fmt.Sprintf("0x%x", DefaultMark), "table", routeTable); err != nil {
		_ = command("wg-quick", "down", generated)
		return err
	}
	if err := command("ip", "route", "replace", "default", "dev", interfaceName, "table", routeTable); err != nil {
		_ = command("ip", "rule", "del", "fwmark", fmt.Sprintf("0x%x", DefaultMark), "table", routeTable)
		_ = command("wg-quick", "down", generated)
		return err
	}
	if err := writeState(DefaultMark); err != nil {
		return err
	}
	return nil
}

func addInterfaceSetting(text, setting string) string {
	idx := strings.Index(text, "[Peer]")
	if idx < 0 {
		return text + "\n" + setting + "\n"
	}
	return text[:idx] + setting + "\n\n" + text[idx:]
}

func Down() error {
	if err := ensureRoot(); err != nil {
		return err
	}
	generated := filepath.Join(filepath.Dir(StatePath()), interfaceName+".conf")
	_ = command("ip", "route", "del", "default", "dev", interfaceName, "table", routeTable)
	_ = command("ip", "rule", "del", "fwmark", fmt.Sprintf("0x%x", DefaultMark), "table", routeTable)
	if _, err := os.Stat(generated); err == nil {
		_ = command("wg-quick", "down", generated)
	}
	return clearState()
}

func Status() (string, error) {
	if ActiveMark() == 0 {
		return "off", nil
	}
	cmd := exec.Command("wg", "show", interfaceName, "latest-handshakes")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "configured, tunnel check failed: " + strings.TrimSpace(string(out)), nil
	}
	return "on\n" + strings.TrimSpace(string(out)), nil
}

func DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	d := net.Dialer{}
	mark := ActiveMark()
	if mark > 0 {
		d.Control = func(_, _ string, c syscall.RawConn) error {
			var controlErr error
			if err := c.Control(func(fd uintptr) {
				controlErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_MARK, mark)
			}); err != nil {
				return err
			}
			return controlErr
		}
	}
	return d.DialContext(ctx, network, address)
}
