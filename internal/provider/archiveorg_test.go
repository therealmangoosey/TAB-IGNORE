package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

func TestArchiveOrgSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/advancedsearch.php" {
			json.NewEncoder(w).Encode(map[string]any{
				"response": map[string]any{
					"docs": []map[string]any{
						{"identifier": "night_of_the_living_dead_1968", "title": "Night of the Living Dead", "year": 1968, "mediatype": "movies"},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	a := NewArchiveOrg(server.Client(), server.URL)
	hits, err := a.Search(context.Background(), "living dead", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].Title != "Night of the Living Dead" {
		t.Fatalf("unexpected hits: %+v", hits)
	}
}

func TestArchiveOrgResolve(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metadata/abc123" {
			json.NewEncoder(w).Encode(map[string]any{
				"server": "ia123.us.archive.org",
				"dir":    "/9/items/abc123",
				"files": []map[string]any{
					{"name": "abc123.mp4", "format": "MPEG4", "size": 1234, "source": "original"},
					{"name": "abc123.txt", "format": "Text", "size": 10, "source": "original"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	a := NewArchiveOrg(server.Client(), server.URL)
	srcs, err := a.Resolve(context.Background(), hermit.Ref{Kind: hermit.KindMovie, IMDBID: "abc123"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(srcs) != 1 {
		t.Fatalf("expected 1 source, got %d", len(srcs))
	}
	if srcs[0].SizeBytes != 1234 {
		t.Fatalf("size: %d", srcs[0].SizeBytes)
	}
}
