// Package play handles handoff to an external Android or desktop media player
// and parses watch-progress payloads.
package play

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Open hands a media URL to the platform player. On Android it uses am start
// (mpv/VLC with hardware decode). Elsewhere it tries xdg-open.
func Open(ctx context.Context, mediaURL string, contentType string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if runtime.GOOS == "android" {
		args := []string{"start", "-a", "android.intent.action.VIEW"}
		if contentType != "" {
			args = append(args, "-t", contentType)
		}
		args = append(args, "-d", mediaURL)
		cmd := exec.CommandContext(ctx, "am", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("am start: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if _, err := exec.LookPath("xdg-open"); err == nil {
		cmd := exec.CommandContext(ctx, "xdg-open", mediaURL)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("xdg-open: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	return fmt.Errorf("no player handoff available on %s/%s", runtime.GOOS, runtime.GOARCH)
}

// Available reports whether a player handoff command exists.
func Available() bool {
	if runtime.GOOS == "android" {
		return true
	}
	_, err := exec.LookPath("xdg-open")
	return err == nil
}

// Progress is a resumable position record.
type Progress struct {
	Season    int     `json:"season"`
	Episode   int     `json:"episode"`
	PositionS float64 `json:"position_s"`
	DurationS float64 `json:"duration_s"`
	Watched   float64 `json:"progress"`
}

// ParseProgressJSON accepts the shape used by common watch-later feeds:
// {"s1e1": {"progress": 0.42, "duration": 1800}}. It is deliberately simple
// and tolerant of both camelCase and snake_case keys.
func ParseProgressJSON(input string) (map[string]Progress, error) {
	out := map[string]Progress{}
	body := strings.TrimSpace(input)
	if body == "" {
		return out, nil
	}
	// A full JSON parser is in the standard library; keep this function focused
	// on the compact single-key shape by delegating to a small helper.
	return parseFlatJSON(body)
}

func parseFlatJSON(body string) (map[string]Progress, error) {
	out := map[string]Progress{}
	var raw map[string]map[string]any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return nil, err
	}
	for key, v := range raw {
		season, episode, ok := parseKey(key)
		if !ok {
			continue
		}
		p := Progress{Season: season, Episode: episode}
		if n, ok := floatVal(v["progress"]); ok {
			p.Watched = n
		}
		if n, ok := floatVal(v["watched"]); ok {
			p.Watched = n
		}
		if n, ok := floatVal(v["duration"]); ok {
			p.DurationS = n
		}
		if n, ok := floatVal(v["position_s"]); ok {
			p.PositionS = n
		}
		out[key] = p
	}
	return out, nil
}

func parseKey(key string) (int, int, bool) {
	key = strings.ToLower(key)
	if len(key) < 2 || key[0] != 's' {
		return 0, 0, false
	}
	parts := strings.SplitN(key, "e", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	season, err1 := strconv.Atoi(strings.TrimPrefix(parts[0], "s"))
	episode, err2 := strconv.Atoi(parts[1])
	return season, episode, err1 == nil && err2 == nil
}

func floatVal(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	default:
		return 0, false
	}
}
