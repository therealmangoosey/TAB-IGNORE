package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

func TestGetMediaByTMDBRespectsKind(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "hermit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.SaveMedia(ctx, hermit.Media{Kind: hermit.KindMovie, TMDBID: 77, Title: "Movie"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.GetMediaByTMDB(ctx, 77, hermit.KindTV); err == nil {
		t.Fatal("expected TV lookup not to return movie cache")
	}
}

func TestListJobsHidesFutureRetries(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "hermit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	id, err := d.InsertJob(ctx, hermit.Job{Provider: "genericm3u8", Quality: hermit.QualityAuto, State: hermit.JobQueued, CreatedAt: time.Now(), NextRetryAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := d.ListJobs(ctx, []string{string(hermit.JobQueued)}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("future retry job %d was returned: %+v", id, jobs)
	}
}
