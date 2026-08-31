package fetch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

func TestDownloadDirect(t *testing.T) {
	payload := make([]byte, 1024*1024)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	var wantHash [32]byte = sha256.Sum256(payload)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "file.bin", time.Time{}, bytes.NewReader(payload))
	}))
	defer server.Close()

	dl := NewDownloader(server.Client(), 1<<22, 2)
	dest := filepath.Join(t.TempDir(), "out.bin")
	res, err := dl.Download(context.Background(), hermit.Source{
		Provider: "test", Kind: hermit.TransportDirect, URL: server.URL,
	}, dest)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if res.Bytes != int64(len(payload)) {
		t.Fatalf("bytes: %d want %d", res.Bytes, len(payload))
	}
	if res.SHA256 != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("hash mismatch")
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(payload) {
		t.Fatalf("len mismatch")
	}
}

func TestDownloadHLS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/master.m3u8":
			w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000,RESOLUTION=640x360,CODECS=avc1\nmedia.m3u8\n"))
		case "/media.m3u8":
			w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2,\nseg0.ts\n#EXTINF:2,\nseg1.ts\n"))
		case "/seg0.ts":
			w.Write([]byte("abc"))
		case "/seg1.ts":
			w.Write([]byte("def"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dl := NewDownloader(server.Client(), 1<<20, 1)
	dest := filepath.Join(t.TempDir(), "out.ts")
	res, err := dl.Download(context.Background(), hermit.Source{
		Provider: "genericm3u8", Kind: hermit.TransportHLS, URL: server.URL + "/master.m3u8",
	}, dest)
	if err != nil {
		t.Fatalf("hls: %v", err)
	}
	if res.Bytes != 6 {
		t.Fatalf("bytes: %d", res.Bytes)
	}
}
