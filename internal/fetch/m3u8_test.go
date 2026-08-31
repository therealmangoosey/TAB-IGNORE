package fetch

import "testing"

func TestParseMediaPlaylist(t *testing.T) {
	p, err := ParsePlaylist([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:4\n#EXTINF:4,\nseg1.ts\n#EXTINF:4,\nseg2.ts\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !p.Media || len(p.Segments) != 2 {
		t.Fatalf("unexpected playlist: %+v", p)
	}
}

func TestChooseVariantSkipsAV1(t *testing.T) {
	p, err := ParsePlaylist([]byte(`#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=1000000,RESOLUTION=1280x720,CODECS="avc1"
720.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2000000,RESOLUTION=1920x1080,CODECS="av01"
1080av1.m3u8
`))
	if err != nil {
		t.Fatal(err)
	}
	v, err := p.ChooseVariant()
	if err != nil {
		t.Fatal(err)
	}
	if v.URL != "720.m3u8" {
		t.Fatalf("expected avc variant, got %+v", v)
	}
}

func TestResolveURL(t *testing.T) {
	if got := ResolveURL("https://example.com/master.m3u8", "seg1.ts"); got != "https://example.com/seg1.ts" {
		t.Fatalf("relative: %s", got)
	}
	if got := ResolveURL("https://example.com/path/master.m3u8", "https://other/x.ts"); got != "https://other/x.ts" {
		t.Fatalf("absolute: %s", got)
	}
}
