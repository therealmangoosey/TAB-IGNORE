// Package label is the single formatter for media labels, filenames, log
// lines, and redacted JSON. Keeping it in one place prevents the TUI, JSON
// output, logs, and the disk from ever disagreeing.
package label

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

var (
	illegalRe = regexp.MustCompile(`[\\/:*?"<>|]`)
	spaceRe   = regexp.MustCompile(`\s+`)
	emojiRe   = regexp.MustCompile(`[\x{1F000}-\x{1FAFF}\x{2600}-\x{27BF}\x{FE0F}]`)
)

// Clean sanitizes a single path component. It is safe for exFAT and Android.
func Clean(s string) string {
	s = emojiRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\u200b", "")
	s = illegalRe.ReplaceAllString(s, "-")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Other, r) {
			return -1
		}
		return r
	}, s)
	s = spaceRe.ReplaceAllString(s, " ")
	s = strings.Trim(s, " .")
	return s
}

// Filename formats a TV episode filename.
func Filename(showTitle string, season, episode int, episodeTitle, ext string) string {
	title := Clean(episodeTitle)
	if title == "" {
		title = "Episode"
	}
	component := Clean(showTitle) + " - S" + pad(season) + "E" + pad(episode) + " - " + truncateTitle(title, 64)
	ext = strings.TrimPrefix(strings.ToLower(ext), ".")
	if ext == "" {
		ext = "mp4"
	}
	return component + "." + ext
}

// MovieFilename formats a movie filename.
func MovieFilename(title string, year int, ext string) string {
	t := Clean(title)
	if year > 0 {
		t = t + " (" + Itoa(year) + ")"
	}
	ext = strings.TrimPrefix(strings.ToLower(ext), ".")
	if ext == "" {
		ext = "mp4"
	}
	return t + "." + ext
}

func pad(n int) string {
	if n < 10 {
		return "0" + Itoa(n)
	}
	return Itoa(n)
}

func truncateTitle(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	// Cut at a word boundary and never add an ellipsis.
	end := maxRunes
	for end > 0 && !unicode.IsSpace(r[end-1]) && !unicode.IsLetter(r[end-1]) && !unicode.IsDigit(r[end-1]) {
		end--
	}
	boundary := strings.LastIndexFunc(string(r[:end]), unicode.IsSpace)
	if boundary > maxRunes/2 {
		return strings.TrimSpace(string(r[:boundary]))
	}
	return strings.TrimSpace(string(r[:end]))
}

// Itoa is a tiny helper used to keep the runtime dependency set small.
func Itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// Line returns the screen-facing label for a show/episode row. It deliberately
// contains no provider, host, quality, or URL information.
func Line(showTitle string, season, episode int, episodeTitle string) string {
	s := Clean(showTitle)
	if season > 0 || episode > 0 {
		s += " — S" + pad(season) + "E" + pad(episode)
	}
	if title := Clean(episodeTitle); title != "" {
		s += " · " + title
	}
	return s
}

// Redact replaces provider/host names with stable pseudonyms. It is applied to
// JSON output, logs, and doctor bundles unless --debug is explicitly set.
// Unknown strings are left untouched so internal errors remain readable.
func Redact(s string, providerHosts map[string]string) string {
	if len(providerHosts) == 0 {
		return s
	}
	hostNum := 0
	providers := map[string]bool{}
	for _, host := range providerHosts {
		if host == "" {
			continue
		}
		providers[host] = true
	}
	for host := range providers {
		hostNum++
		repl := "h" + Itoa(hostNum)
		s = strings.ReplaceAll(s, host, repl)
		if u, err := url.Parse(host); err == nil && u.Host != "" {
			s = strings.ReplaceAll(s, u.Host, repl)
		}
	}
	return s
}
