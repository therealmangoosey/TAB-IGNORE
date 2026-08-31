package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

func TestMigrationsApply(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "hermit.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	// Migration 2 should be a no-op if calling Migrate twice.
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestSaveAndGetMedia(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "hermit.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	m := hermit.Media{Kind: hermit.KindTV, TMDBID: 95396, Title: "Severance", SeasonCount: 2, EpisodeCount: 21, Year: 2022}
	id, err := d.SaveMedia(ctx, m)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}
	got, err := d.GetMediaByTMDB(ctx, 95396, hermit.KindTV)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != id || got.Title != "Severance" {
		t.Fatalf("unexpected media: %+v", got)
	}
}

func TestInsertAndListJob(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "hermit.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	m := hermit.Media{Kind: hermit.KindMovie, TMDBID: 1, Title: "Test"}
	id, _ := d.SaveMedia(ctx, m)
	j := hermit.Job{MediaID: id, Season: 1, Episode: 1, Provider: "genericm3u8", State: hermit.JobQueued}
	jobID, err := d.InsertJob(ctx, j)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	jobs, err := d.ListJobs(ctx, []string{string(hermit.JobQueued)}, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != jobID {
		t.Fatalf("unexpected jobs: %+v", jobs)
	}
}
