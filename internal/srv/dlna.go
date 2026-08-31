package srv

import (
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	dlna "github.com/anacrolix/dms/dlna"
	dms "github.com/anacrolix/dms/dlna/dms"
	analog "github.com/anacrolix/log"
	"github.com/anacrolix/dms/upnpav"
	"github.com/therealmangoosey/TAB-IGNORE/internal/lib"
)

const dlnaHTTPAddrDefault = "0.0.0.0:8789"
const fallbackIconPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

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
	if d.library == nil || d.library.Root == "" {
		if logf != nil {
			logf("DLNA disabled: library path is empty")
		}
		return
	}
	root, err := filepath.Abs(d.library.Root)
	if err != nil {
		if logf != nil {
			logf("DLNA library path invalid: " + err.Error())
		}
		return
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		if logf != nil {
			logf("DLNA library setup failed: " + err.Error())
		}
		return
	}
	ln, err := net.Listen("tcp", d.addr)
	if err != nil {
		if logf != nil {
			logf("DLNA listen failed: " + err.Error())
		}
		return
	}
	logger := analog.NewLogger("hermit", "dlna")
	logger.SetHandlers(analog.DiscardHandler)
	iconBytes, _ := base64.StdEncoding.DecodeString(fallbackIconPNGBase64)
	s := &dms.Server{
		HTTPConn: ln,
		FriendlyName: d.name,
		RootObjectPath: root,
		NoProbe: !commandExists("ffprobe"),
		NoTranscode: !commandExists("ffmpeg"),
		StallEventSubscribe: true,
		IgnoreHidden: true,
		IgnoreUnreadable: true,
		NotifyInterval: 30 * time.Second,
		AllowedIpNets: localInterfaceNetworks(),
		Icons: []dms.Icon{{Width: 1, Height: 1, Depth: 24, Mimetype: "image/png", Bytes: iconBytes}},
		OnBrowseDirectChildren: d.browseDirectChildren,
		OnBrowseMetadata: d.browseMetadata,
		Logger: logger,
	}
	if err := s.Init(); err != nil {
		_ = ln.Close()
		if logf != nil {
			logf("DLNA init failed: " + err.Error())
		}
		return
	}
	s.RootObjectPath = root
	d.server = s
	if logf != nil {
		logf("DLNA media server listening on " + ln.Addr().String() + " as " + d.name)
	}
	done := make(chan error, 1)
	go func() { done <- s.Run() }()
	go func() { <-ctx.Done(); _ = s.Close() }()
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

func mustRoot(root string) string {
	p, _ := filepath.Abs(root)
	return p
}

func hasTraversal(raw string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(raw), "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

func (d *dlnaServer) browseRoot() (string, error) {
	return filepath.Abs(d.library.Root)
}

func (d *dlnaServer) browsePath(objectID string) (string, string, error) {
	root, err := d.browseRoot()
	if err != nil {
		return "", "", err
	}
	pathID, err := url.QueryUnescape(objectID)
	if err != nil {
		return "", "", err
	}
	if pathID == "" || pathID == "0" || pathID == "/" {
		return root, "/", nil
	}
	if hasTraversal(pathID) {
		return "", "", fmt.Errorf("object path escapes library")
	}
	pathID = filepath.ToSlash(filepath.Clean("/" + pathID))
	if pathID == "/" || hasTraversal(pathID) {
		return "", "", fmt.Errorf("object path escapes library")
	}
	full := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(pathID, "/")))
	rootAbs, _ := filepath.Abs(root)
	fullAbs, _ := filepath.Abs(full)
	if fullAbs != rootAbs && !strings.HasPrefix(fullAbs, rootAbs+string(filepath.Separator)) {
		return "", "", fmt.Errorf("object path escapes library")
	}
	return fullAbs, pathID, nil
}

func objectID(pathID string) string {
	if pathID == "/" || pathID == "" {
		return "0"
	}
	return pathID
}

func parentID(pathID string) string {
	if pathID == "/" || pathID == "" {
		return "-1"
	}
	p := filepath.ToSlash(filepath.Dir(filepath.FromSlash(pathID)))
	if p == "." || p == "" {
		p = "/"
	}
	if p == "/" {
		return "0"
	}
	return p
}

func mediaMIME(path string) string {
	m := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if m != "" {
		return m
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mkv":
		return "video/x-matroska"
	case ".ts":
		return "video/mp2t"
	default:
		return "application/octet-stream"
	}
}

func isMedia(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".mp4" || ext == ".m4v" || ext == ".mkv" || ext == ".webm" || ext == ".ts"
}

func (d *dlnaServer) makeObject(pathID string, info os.FileInfo, host string) (interface{}, error) {
	obj := upnpav.Object{ID: objectID(pathID), ParentID: parentID(pathID), Restricted: 1, Title: info.Name(), Date: upnpav.Timestamp{Time: info.ModTime()}}
	if info.IsDir() {
		count := 0
		entries, err := os.ReadDir(filepath.Join(mustRoot(d.library.Root), filepath.FromSlash(strings.TrimPrefix(pathID, "/"))))
		if err == nil {
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".") {
					continue
				}
				if entry.IsDir() || isMedia(entry.Name()) {
					count++
				}
			}
		}
		obj.Class = "object.container.storageFolder"
		return upnpav.Container{Object: obj, ChildCount: count}, nil
	}
	if !info.Mode().IsRegular() || !isMedia(info.Name()) {
		return nil, nil
	}
	obj.Class = "object.item.videoItem"
	resourceURL := (&url.URL{Scheme: "http", Host: host, Path: "/res", RawQuery: url.Values{"path": {pathID}}.Encode()}).String()
	return upnpav.Item{Object: obj, Res: []upnpav.Resource{{
		URL: resourceURL,
		ProtocolInfo: fmt.Sprintf("http-get:*:%s:%s", mediaMIME(info.Name()), dlna.ContentFeatures{SupportRange: true}.String()),
		Size: uint64(info.Size()),
	}}}, nil
}

func (d *dlnaServer) browseDirectChildren(objectPath, rootObjectPath, host, _ string) ([]interface{}, error) {
	root, err := d.browseRoot()
	if err != nil {
		return nil, err
	}
	_ = rootObjectPath
	if hasTraversal(objectPath) {
		return nil, fmt.Errorf("object path escapes library")
	}
	pathID := filepath.ToSlash(filepath.Clean("/" + objectPath))
	full := root
	if pathID != "/" {
		full = filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(pathID, "/")))
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}
	type namedInfo struct{ path string; info os.FileInfo }
	items := make([]namedInfo, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil || (!info.IsDir() && !isMedia(info.Name())) {
			continue
		}
		childID := filepath.ToSlash(filepath.Join(pathID, entry.Name()))
		items = append(items, namedInfo{childID, info})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].info.IsDir() != items[j].info.IsDir() {
			return items[i].info.IsDir()
		}
		return strings.ToLower(items[i].info.Name()) < strings.ToLower(items[j].info.Name())
	})
	ret := make([]interface{}, 0, len(items))
	for _, item := range items {
		obj, err := d.makeObject(item.path, item.info, host)
		if err != nil || obj == nil {
			continue
		}
		ret = append(ret, obj)
	}
	return ret, nil
}

func (d *dlnaServer) browseMetadata(objectPath, rootObjectPath, host, _ string) (interface{}, error) {
	full, pathID, err := d.browsePath(objectPath)
	if err != nil {
		return nil, err
	}
	_ = rootObjectPath
	info, err := os.Stat(full)
	if err != nil {
		return nil, err
	}
	if pathID == "/" {
		obj := upnpav.Object{ID: "0", ParentID: "-1", Restricted: 1, Title: d.name, Class: "object.container.storageFolder", Date: upnpav.Timestamp{Time: info.ModTime()}}
		return upnpav.Container{Object: obj, ChildCount: lenOrZero(d.browseChildCount(full))}, nil
	}
	return d.makeObject(pathID, info, host)
}

func (d *dlnaServer) browseChildCount(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }

func lenOrZero(entries []os.DirEntry, err error) int {
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.IsDir() || isMedia(entry.Name()) {
			count++
		}
	}
	return count
}
