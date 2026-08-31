package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/therealmangoosey/TAB-IGNORE/internal/config"
	"github.com/therealmangoosey/TAB-IGNORE/internal/vpn"
)

func cmdVPN(ctx context.Context, cfg config.Config, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: hmt vpn up <wireguard.conf> | hmt vpn down | hmt vpn status | hmt vpn start <wireguard.conf>")
	}

	switch args[0] {
	case "up":
		if len(args) != 2 {
			return errors.New("usage: hmt vpn up <wireguard.conf>")
		}
		if err := vpn.Up(args[1]); err != nil {
			return err
		}
		fmt.Println("VPN on: hmt network sockets are now split-routed through WireGuard")
		return nil
	case "down":
		if err := vpn.Down(); err != nil {
			return err
		}
		fmt.Println("VPN off: hmt uses the normal network route")
		return nil
	case "status":
		status, err := vpn.Status()
		if err != nil {
			return err
		}
		fmt.Println(status)
		return nil
	case "start":
		if len(args) != 2 {
			return errors.New("usage: hmt vpn start <wireguard.conf>")
		}
		if os.Geteuid() != 0 {
			return errors.New("hmt vpn start must run as root so the daemon can mark its sockets")
		}
		if err := vpn.Up(args[1]); err != nil {
			return err
		}
		return daemonRun(ctx, cfg)
	default:
		return fmt.Errorf("unknown vpn command %q; use up, down, start, or status", args[0])
	}
}
