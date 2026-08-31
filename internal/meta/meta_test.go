package meta

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/internal/db"
	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

func TestShowUsesFreshCache(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "hermit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	want := hermit.Media{Kind: hermit.KindTV, TMDBID: 123, Title: "Cached", MetaFetchedAt: time.Now()}
	if _, err := d.SaveMedia(ctx, want); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("unexpected TMDB request for fresh cache")
	})}
	c := NewClientWithTTL(d, client, "key", time.Hour)
	got, seasons, episodes, err := c.Show(ctx, hermit.Ref{Kind: hermit.KindTV, TMDBID: 123})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Cached" || seasons != nil || episodes != nil {
		t.Fatalf("unexpected cached result: %+v seasons=%v episodes=%v", got, seasons, episodes)
	}
}

func TestSeasonsAndEpisodesChecksStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	d, err := db.Open(filepath.Join(t.TempDir(), "hermit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	c := NewClient(d, server.Client(), "key")
	c.Base = server.URL
	if _, _, err := c.seasonsAndEpisodes(context.Background(), hermit.Ref{TMDBID: 123}, 1); err == nil {
		t.Fatal("expected HTTP error from season details")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
