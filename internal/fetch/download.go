package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

// Result is the outcome of a download.
type Result struct {
	Path   string
	Bytes  int64
	SHA256 string
	Parts  int
	HLS    bool
}

// Downloader performs bounded, resumable downloads.
type Downloader struct {
	Client      *http.Client
	Rate        *BoundedRate
	PartSize    int64
	Concurrency int
}

// NewDownloader creates a bounded downloader.
func NewDownloader(client *http.Client, maxBytesPerSec int64, concurrency int) *Downloader {
	if concurrency <= 0 {
		concurrency = 4
	}
	if concurrency > 8 {
		concurrency = 8
	}
	if maxBytesPerSec <= 0 {
		maxBytesPerSec = 4 * 1024 * 1024
	}
	return &Downloader{Client: client, Rate: NewBoundedRate(maxBytesPerSec), PartSize: 8 << 20, Concurrency: concurrency}
}

// Download fetches a source into destFile. If destFile already has a sibling
// parts directory, incomplete parts are resumed.
func (d *Downloader) Download(ctx context.Context, src hermit.Source, destFile string) (Result, error) {
	if strings.HasPrefix(src.URL, "file://") {
		return d.downloadFile(ctx, src.URL, destFile)
	}
	if src.Kind == hermit.TransportHLS {
		return d.downloadHLS(ctx, src, destFile)
	}
	return d.downloadDirect(ctx, src, destFile)
}

func (d *Downloader) downloadFile(ctx context.Context, raw, destFile string) (Result, error) {
	path := strings.TrimPrefix(raw, "file://")
	in, err := os.Open(path)
	if err != nil {
		return Result{}, fmt.Errorf("open local source: %w", err)
	}
	defer in.Close()
	out, err := createDest(destFile)
	if err != nil {
		return Result{}, err
	}
	defer out.Close()
	h := sha256.New()
	buf := make([]byte, 256*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			d.Rate.Wait(int64(n))
			if _, werr := out.Write(buf[:n]); werr != nil {
				return Result{}, werr
			}
			h.Write(buf[:n])
			total += int64(n)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return Result{}, rerr
		}
	}
	return Result{Path: destFile, Bytes: total, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

func createDest(destFile string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(destFile), 0o755); err != nil {
		return nil, err
	}
	return os.Create(destFile)
}

func (d *Downloader) downloadDirect(ctx context.Context, src hermit.Source, destFile string) (Result, error) {
	total, ranged, err := d.probeTotal(ctx, src)
	if err != nil {
		return Result{}, err
	}
	if !ranged || total <= 0 {
		return d.downloadStream(ctx, src, destFile)
	}

	partDir := destFile + ".parts"
	if err := os.MkdirAll(partDir, 0o755); err != nil {
		return Result{}, err
	}
	size := d.PartSize
	if size <= 0 {
		size = 8 << 20
	}
	count := int((total + size - 1) / size)
	if count <= 0 {
		count = 1
	}
	if count > d.Concurrency*8 {
		count = d.Concurrency * 8
	}
	var partCount int32
	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	jobs := make(chan int)
	for i := 0; i < d.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				start := int64(idx) * size
				end := start + size - 1
				if end >= total {
					end = total - 1
				}
				if start > end {
					continue
				}
				partPath := filepath.Join(partDir, fmt.Sprintf("part.%06d", idx))
				_, err := d.fetchRangeToFile(ctx, src, start, end, partPath, true)
				if err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
				atomic.AddInt32(&partCount, 1)
			}
		}()
	}
	for i := 0; i < count; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	select {
	case err := <-errCh:
		return Result{}, fmt.Errorf("download: %w", err)
	default:
	}

	out, err := createDest(destFile)
	if err != nil {
		return Result{}, err
	}
	h := sha256.New()
	for i := 0; i < count; i++ {
		p, err := os.Open(filepath.Join(partDir, fmt.Sprintf("part.%06d", i)))
		if err != nil {
			out.Close()
			return Result{}, err
		}
		_, err = io.Copy(io.MultiWriter(out, h), p)
		p.Close()
		if err != nil {
			out.Close()
			return Result{}, err
		}
	}
	out.Close()
	os.RemoveAll(partDir)
	return Result{Path: destFile, Bytes: total, SHA256: hex.EncodeToString(h.Sum(nil)), Parts: int(partCount)}, nil
}

func (d *Downloader) probeTotal(ctx context.Context, src hermit.Source) (int64, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return 0, false, err
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := d.Client.Do(req)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode != http.StatusPartialContent {
		return 0, false, nil
	}
	contentRange := resp.Header.Get("Content-Range")
	var total int64
	slash := strings.LastIndex(contentRange, "/")
	if slash >= 0 {
		fmt.Sscanf(strings.TrimSpace(contentRange[slash+1:]), "%d", &total)
	}
	return total, total > 0, nil
}

func (d *Downloader) fetchRangeToFile(ctx context.Context, src hermit.Source, start, end int64, path string, _ bool) (int64, error) {
	var done int64
	if info, err := os.Stat(path); err == nil {
		done = info.Size()
	}
	start += done
	if start > end {
		return done, nil
	}
	fh, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	defer fh.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return done, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	if src.Referer != "" {
		req.Header.Set("Referer", src.Referer)
	}
	resp, err := d.Client.Do(req)
	if err != nil {
		return done, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return done, fmt.Errorf("range fetch HTTP %d", resp.StatusCode)
	}
	buf := make([]byte, 256*1024)
	var written int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			d.Rate.Wait(int64(n))
			if _, werr := fh.Write(buf[:n]); werr != nil {
				return done + written, werr
			}
			written += int64(n)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return done + written, rerr
		}
	}
	return done + written, nil
}

func (d *Downloader) downloadStream(ctx context.Context, src hermit.Source, destFile string) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return Result{}, err
	}
	if src.Referer != "" {
		req.Header.Set("Referer", src.Referer)
	}
	resp, err := d.Client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("download HTTP %d", resp.StatusCode)
	}
	out, err := createDest(destFile)
	if err != nil {
		return Result{}, err
	}
	defer out.Close()
	h := sha256.New()
	buf := make([]byte, 256*1024)
	var total int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			d.Rate.Wait(int64(n))
			if _, werr := out.Write(buf[:n]); werr != nil {
				return Result{}, werr
			}
			h.Write(buf[:n])
			total += int64(n)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return Result{}, rerr
		}
	}
	return Result{Path: destFile, Bytes: total, SHA256: hex.EncodeToString(h.Sum(nil)), Parts: 1}, nil
}

// downloadHLS parses a media or master playlist and downloads every segment in
// order, concatenating them into a single .ts/.m4s transport file before the
// optional remux step.
func (d *Downloader) downloadHLS(ctx context.Context, src hermit.Source, destFile string) (Result, error) {
	play, err := d.fetchPlaylist(ctx, src.URL, src)
	if err != nil {
		return Result{}, err
	}
	base := src.URL
	if len(play.Variants) > 0 {
		v, err := play.ChooseVariant()
		if err != nil {
			return Result{}, err
		}
		mediaURL := ResolveURL(src.URL, v.URL)
		play, err = d.fetchPlaylist(ctx, mediaURL, src)
		if err != nil {
			return Result{}, err
		}
		base = mediaURL
	}
	if len(play.Segments) == 0 {
		return Result{}, fmt.Errorf("HLS playlist has no segments")
	}
	out, err := createDest(destFile)
	if err != nil {
		return Result{}, err
	}
	defer out.Close()
	h := sha256.New()
	buf := make([]byte, 256*1024)
	var total int64
	for i, segRaw := range play.Segments {
		segURL := ResolveURL(base, segRaw)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, segURL, nil)
		if err != nil {
			return Result{}, err
		}
		if src.Referer != "" {
			req.Header.Set("Referer", src.Referer)
		}
		resp, err := d.Client.Do(req)
		if err != nil {
			return Result{}, fmt.Errorf("segment %d: %w", i, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return Result{}, fmt.Errorf("segment %d HTTP %d", i, resp.StatusCode)
		}
		for {
			n, rerr := resp.Body.Read(buf)
			if n > 0 {
				d.Rate.Wait(int64(n))
				if _, werr := out.Write(buf[:n]); werr != nil {
					resp.Body.Close()
					return Result{}, werr
				}
				h.Write(buf[:n])
				total += int64(n)
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				resp.Body.Close()
				return Result{}, rerr
			}
		}
		resp.Body.Close()
	}
	return Result{Path: destFile, Bytes: total, SHA256: hex.EncodeToString(h.Sum(nil)), HLS: true, Parts: len(play.Segments)}, nil
}

func (d *Downloader) fetchPlaylist(ctx context.Context, raw string, src hermit.Source) (*Playlist, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	if src.Referer != "" {
		req.Header.Set("Referer", src.Referer)
	}
	resp, err := d.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("playlist HTTP %d", resp.StatusCode)
	}
	body, err := ReadPlaylist(resp.Body)
	if err != nil {
		return nil, err
	}
	return ParsePlaylist(body)
}
