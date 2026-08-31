package fetch

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
)

// Variant is one rendition in an HLS master playlist.
type Variant struct {
	URL          string
	Resolution   string
	Bandwidth    int
	Codec        string
	Audio        string
}

// Playlist is a parsed HLS playlist. A playlist is either a media playlist
// (Segments non-empty) or a master playlist (Variants non-empty).
type Playlist struct {
	Media     bool
	Segments  []string
	Variants  []Variant
	TargetDur float64
}

// ParsePlaylist parses either a media or master playlist.
func ParsePlaylist(data []byte) (*Playlist, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	p := &Playlist{}
	var pending Variant
	hasPending := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "#EXT-X-MEDIA"):
			p.Media = false
		case strings.HasPrefix(line, "#EXT-X-TARGETDURATION:"):
			_, _ = fmt.Sscanf(strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:"), "%f", &p.TargetDur)
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			hasPending = true
			pending = Variant{}
			for _, part := range strings.Split(strings.TrimPrefix(line, "#EXT-X-STREAM-INF:"), ",") {
				k, v, ok := strings.Cut(part, "=")
				if !ok {
					continue
				}
				k = strings.TrimSpace(k)
				v = strings.Trim(strings.TrimSpace(v), "\"")
				switch k {
				case "RESOLUTION":
					pending.Resolution = v
				case "BANDWIDTH":
					_, _ = fmt.Sscanf(v, "%d", &pending.Bandwidth)
				case "CODECS":
					pending.Codec = v
				case "AUDIO":
					pending.Audio = v
				}
			}
		case hasPending:
			pending.URL = line
			p.Variants = append(p.Variants, pending)
			hasPending = false
		case line[0] != '#':
			p.Segments = append(p.Segments, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	p.Media = len(p.Segments) > 0 && len(p.Variants) == 0
	if len(p.Segments) > 0 && len(p.Variants) > 0 {
		return nil, fmt.Errorf("playlist contains both segments and variants")
	}
	return p, nil
}

// ChooseVariant picks the best non-AV1 rendition, preferring 1080p and then
// bandwidth. AV1 is used only when no non-AV1 variant exists because the target
// Android hardware profile is designed around hardware H.264/HEVC decode.
func (p *Playlist) ChooseVariant() (Variant, error) {
	if len(p.Variants) == 0 {
		return Variant{}, fmt.Errorf("playlist has no variants")
	}
	eligible := p.Variants[:0]
	for _, v := range p.Variants {
		if !strings.Contains(strings.ToLower(v.Codec), "av01") && !strings.Contains(strings.ToLower(v.Codec), "av1") {
			eligible = append(eligible, v)
		}
	}
	if len(eligible) == 0 {
		eligible = p.Variants
	}

	best := eligible[0]
	bestScore := -1
	for _, v := range eligible {
		score := v.Bandwidth / 10000
		if strings.Contains(v.Resolution, "1920x1080") || strings.Contains(v.Resolution, "1080") {
			score += 100
		} else if strings.Contains(v.Resolution, "1280x720") || strings.Contains(v.Resolution, "720") {
			score += 70
		}
		if score > bestScore {
			bestScore = score
			best = v
		}
	}
	return best, nil
}

// ResolveURL resolves a relative URL against a base URL.
func ResolveURL(base, ref string) string {
	if isAbsolute(ref) {
		return ref
	}
	base = strings.TrimRight(base, "/")
	idx := strings.LastIndex(base, "/")
	if idx < 0 {
		return base + "/" + ref
	}
	return base[:idx+1] + ref
}

func isAbsolute(ref string) bool {
	return strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "file://")
}

// ReadPlaylist reads a bounded playlist body (maximum 4 MiB).
func ReadPlaylist(r io.Reader) ([]byte, error) {
	const max = 4 << 20
	var b bytes.Buffer
	if _, err := io.Copy(&b, io.LimitReader(r, max+1)); err != nil {
		return nil, err
	}
	if b.Len() > max {
		return nil, fmt.Errorf("playlist too large")
	}
	return b.Bytes(), nil
}
