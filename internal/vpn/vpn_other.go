//go:build !linux

package vpn

import (
	"context"
	"errors"
	"net"
)

func Up(string) error {
	return errors.New("split-tunnel VPN currently requires Linux/Android root networking")
}

func Down() error {
	return errors.New("split-tunnel VPN currently requires Linux/Android root networking")
}

func Status() (string, error) {
	if ActiveMark() == 0 {
		return "off", nil
	}
	return "configured, unsupported platform", nil
}

func DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, address)
}
