package play

import "testing"

func TestParseProgressJSON(t *testing.T) {
	p, err := ParseProgressJSON(`{"s1e2":{"progress":0.4,"duration":1800}}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 1 {
		t.Fatalf("len: %d", len(p))
	}
	pr := p["s1e2"]
	if pr.Season != 1 || pr.Episode != 2 || pr.Watched != 0.4 || pr.DurationS != 1800 {
		t.Fatalf("unexpected: %+v", pr)
	}
}
