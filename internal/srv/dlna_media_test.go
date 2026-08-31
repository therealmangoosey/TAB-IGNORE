package srv

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/therealmangoosey/TAB-IGNORE/internal/lib"
)

func TestDLNAMediaServesRangeWithMIME(t *testing.T) {
	root := t.TempDir()
	library := lib.New(root, "", "")
	path := filepath.Join(root, "Show", "Episode.mp4")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := &dlnaServer{library: library}
	req := httptest.NewRequest("GET", "/media?path=%2FShow%2FEpisode.mp4", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Range", "bytes=2-5")
	w := httptest.NewRecorder()
	d.serveMedia(w, req)
	if w.Code != 206 { t.Fatalf("status = %d, want 206", w.Code) }
	if got := w.Header().Get("Content-Type"); got != "video/mp4" { t.Fatalf("content type = %q, want video/mp4", got) }
	if got := w.Body.String(); got != "2345" { t.Fatalf("body = %q, want 2345", got) }
	if got := w.Header().Get("Accept-Ranges"); got != "bytes" { t.Fatalf("accept-ranges = %q, want bytes", got) }
}
