// Package mux performs copy-only remuxing and faststart checks. It never
// encodes video or audio; the only transformation is MP4 faststart and
// metadata scrubbing through ffmpeg's copy mode.
package mux

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Available reports whether ffmpeg is installed.
func Available() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// NeedsFaststart parses the MP4 box tree and reports whether moov appears
// after mdat (which prevents HTTP range seeking without reading everything).
func NeedsFaststart(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if !hasMP4Header(f) {
		return false, nil
	}
	moovPos, mdatPos, err := boxPositions(path)
	if err != nil {
		return false, err
	}
	if moovPos < 0 {
		// No moov means the file is not a valid MP4; let ffmpeg decide.
		return true, nil
	}
	return moovPos > mdatPos && mdatPos >= 0, nil
}

func hasMP4Header(f *os.File) bool {
	data := make([]byte, 12)
	if _, err := f.ReadAt(data, 0); err != nil {
		return false
	}
	return bytes.Equal(data[4:8], []byte("ftyp"))
}

func boxPositions(path string) (int64, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return -1, -1, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return -1, -1, err
	}
	fileSize := info.Size()
	var offset, moovPos, mdatPos int64 = 0, -1, -1
	for offset < fileSize {
		hdr := make([]byte, 8)
		if _, err := f.ReadAt(hdr, offset); err != nil {
			return moovPos, mdatPos, nil
		}
		size := int64(binary.BigEndian.Uint32(hdr[:4]))
		headerSize := int64(8)
		if size == 1 {
			ext := make([]byte, 8)
			if _, err := f.ReadAt(ext, offset+8); err != nil {
				return moovPos, mdatPos, nil
			}
			size = int64(binary.BigEndian.Uint64(ext))
			headerSize = 16
			if size < headerSize {
				return moovPos, mdatPos, nil
			}
		} else if size == 0 {
			size = fileSize - offset
		}
		if size < headerSize || size > fileSize-offset {
			return moovPos, mdatPos, nil
		}
		name := string(hdr[4:8])
		if name == "moov" {
			moovPos = offset
		}
		if name == "mdat" {
			mdatPos = offset
		}
		if moovPos >= 0 && mdatPos >= 0 {
			return moovPos, mdatPos, nil
		}
		offset += size
	}
	return moovPos, mdatPos, nil
}

// Remux copies the input into the output, adds faststart, and scrubs global
// metadata. It returns false if ffmpeg is unavailable or the input is already
// faststart (in which case the file is used as-is).
func Remux(in, out, title, album, genre, date string, allowMeta bool) (bool, error) {
	if !Available() {
		return false, nil
	}
	need, err := NeedsFaststart(in)
	if err != nil {
		return false, err
	}
	if !need && !allowMeta {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return false, err
	}
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-y", "-i", in,
		"-map", "0", "-c", "copy",
	}
	if strings.HasSuffix(strings.ToLower(in), ".ts") {
		args = append(args, "-bsf:a", "aac_adtstoasc")
	}
	args = append(args, "-map_metadata", "-1")
	if title != "" {
		args = append(args, "-metadata", "title="+title)
	}
	if album != "" {
		args = append(args, "-metadata", "album="+album)
	}
	if genre != "" {
		args = append(args, "-metadata", "genre="+genre)
	}
	if date != "" {
		args = append(args, "-metadata", "date="+date)
	}
	if strings.HasSuffix(strings.ToLower(out), ".mp4") {
		args = append(args, "-movflags", "+faststart")
	}
	args = append(args, out)

	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return true, nil
}
