package provider

import (
	"testing"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

func TestScoreSourcePenalizesBannedHost(t *testing.T) {
	r := &Registry{hosts: map[string]HostStats{}}
	url := "https://media.example.test/video.mp4"
	r.hosts["media.example.test"] = HostStats{Origin: "media.example.test", Samples: 4, EWMASuccess: 1, BannedUntil: time.Now().Add(time.Hour)}
	got := scoreSource(hermit.Source{URL: url, Quality: hermit.Quality1080, Codec: hermit.CodecAVC, SizeBytes: 1}, r)
	if got >= 0 {
		t.Fatalf("banned host score = %f, want negative", got)
	}
}

func TestScoreSourceAllowsExpiredBan(t *testing.T) {
	r := &Registry{hosts: map[string]HostStats{}}
	url := "https://media.example.test/video.mp4"
	r.hosts["media.example.test"] = HostStats{Origin: "media.example.test", Samples: 4, EWMASuccess: 1, BannedUntil: time.Now().Add(-time.Hour)}
	got := scoreSource(hermit.Source{URL: url, Quality: hermit.Quality1080, Codec: hermit.CodecAVC, SizeBytes: 1}, r)
	if got <= 0 {
		t.Fatalf("expired-ban score = %f, want positive", got)
	}
}
