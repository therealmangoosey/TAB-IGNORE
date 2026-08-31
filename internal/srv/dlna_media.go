package srv

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	dlna "github.com/anacrolix/dms/dlna"
)

func (d *dlnaServer) serveMedia(w http.ResponseWriter, r *http.Request) {
	if !d.allowedMediaClient(r.RemoteAddr) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	pathID, err := url.QueryUnescape(r.URL.Query().Get("path"))
	if err != nil || pathID == "" || hasTraversal(pathID) {
		http.NotFound(w, r)
		return
	}
	root, err := filepath.Abs(d.library.Root)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rel := strings.TrimPrefix(filepath.ToSlash(filepath.Clean("/"+pathID)), "/")
	full := filepath.Join(root, filepath.FromSlash(rel))
	fullAbs, err := filepath.Abs(full)
	if err != nil || (fullAbs != root && !strings.HasPrefix(fullAbs, root+string(filepath.Separator))) {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(fullAbs)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !isMedia(info.Name()) {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", mediaMIME(info.Name()))
	w.Header().Set(dlna.TransferModeDomain, "Streaming")
	w.Header().Set(dlna.ContentFeaturesDomain, dlna.ContentFeatures{
		SupportTimeSeek: true,
		SupportRange:    true,
	}.String())
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Disposition", "inline; filename="+fmt.Sprintf("%q", info.Name()))
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

func (d *dlnaServer) allowedMediaClient(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, network := range localInterfaceNetworks() {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
