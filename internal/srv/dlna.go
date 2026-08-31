package srv

import (
	"context"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	dms "github.com/anacrolix/dms/dlna/dms"
	analog "github.com/anacrolix/log"
	"github.com/therealmangoosey/TAB-IGNORE/internal/lib"
)

const dlnaHTTPAddrDefault = "0.0.0.0:8789"

type dlnaServer struct {
	library *lib.Library
	name    string
	addr    string
	server  *dms.Server
}

func newDLNAServer(library *lib.Library) *dlnaServer {
	addr := os.Getenv("HERMIT_MEDIA_SERVER_ADDR")
	if addr == "" {
		addr = dlnaHTTPAddrDefault
	}
	name := os.Getenv("HERMIT_MEDIA_SERVER_NAME")
	if name == "" {
		name = "Hermit"
	}
	return &dlnaServer{library: library, name: name, addr: addr}
}

func (d *dlnaServer) start(ctx context.Context, logf func(string)) {
	ln, err := net.Listen("tcp", d.addr)
	if err != nil {
		if logf != nil {
			logf("DLNA listen failed: " + err.Error())
		}
		return
	}

	logger := analog.NewLogger("hermit", "dlna")
	logger.SetHandlers(analog.DiscardHandler)
	s := &dms.Server{
		HTTPConn:            ln,
		FriendlyName:        d.name,
		RootObjectPath:      d.library.Root,
		NoProbe:             !commandExists("ffprobe"),
		NoTranscode:         !commandExists("ffmpeg"),
		StallEventSubscribe: true,
		IgnoreHidden:        true,
		IgnoreUnreadable:    true,
		NotifyInterval:      30 * time.Second,
		AllowedIpNets:       localInterfaceNetworks(),
		Logger:              logger,
	}
	if err := s.Init(); err != nil {
		_ = ln.Close()
		if logf != nil {
			logf("DLNA init failed: " + err.Error())
		}
		return
	}
	d.server = s
	if logf != nil {
		logf("DLNA media server listening on " + ln.Addr().String() + " as " + d.name)
	}

	done := make(chan error, 1)
	go func() { done <- s.Run() }()
	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()
	go func() {
		if err := <-done; err != nil && ctx.Err() == nil && logf != nil {
			logf("DLNA server stopped: " + err.Error())
		}
	}()
}

func commandExists(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	_, err := exec.LookPath(name)
	return err == nil
}

func localInterfaceNetworks() []*net.IPNet {
	var nets []*net.IPNet
	interfaces, err := net.Interfaces()
	if err != nil {
		return fallbackLocalNetworks()
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var network *net.IPNet
			switch v := addr.(type) {
			case *net.IPNet:
				network = &net.IPNet{IP: append(net.IP(nil), v.IP...), Mask: append(net.IPMask(nil), v.Mask...)}
			case *net.IPAddr:
				if v.IP.To4() != nil {
					network = &net.IPNet{IP: v.IP.To4(), Mask: net.CIDRMask(32, 32)}
				} else if v.IP != nil {
					network = &net.IPNet{IP: v.IP, Mask: net.CIDRMask(128, 128)}
				}
			}
			if network == nil || network.IP == nil {
				continue
			}
			duplicate := false
			for _, existing := range nets {
				if existing.String() == network.String() {
					duplicate = true
					break
				}
			}
			if !duplicate {
				nets = append(nets, network)
			}
		}
	}
	if len(nets) == 0 {
		return fallbackLocalNetworks()
	}
	return nets
}

func fallbackLocalNetworks() []*net.IPNet {
	_, ipv4, _ := net.ParseCIDR("192.168.0.0/16")
	_, ipv6, _ := net.ParseCIDR("fe80::/10")
	return []*net.IPNet{ipv4, ipv6}
}
