// Package queue implements the daemon's persisted download job state machine.
package queue

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/internal/config"
	"github.com/therealmangoosey/TAB-IGNORE/internal/disk"
	"github.com/therealmangoosey/TAB-IGNORE/internal/db"
	"github.com/therealmangoosey/TAB-IGNORE/internal/fetch"
	"github.com/therealmangoosey/TAB-IGNORE/internal/mux"
	"github.com/therealmangoosey/TAB-IGNORE/internal/provider"
	"github.com/therealmangoosey/TAB-IGNORE/internal/scrub"
	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

type Queue struct {
	DB       *db.DB
	Registry *provider.Registry
	Fetcher  *fetch.Downloader
	Cfg      config.Config

	mu         sync.Mutex
	running    map[int64]bool
	batteryPct int
	charging   bool
}

func NewQueue(database *db.DB, reg *provider.Registry, dl *fetch.Downloader, cfg config.Config) *Queue {
	if dl == nil {
		max, _ := config.ParseSize(cfg.Power.MaxBytesPerSec)
		dl = fetch.NewDownloader(fetch.NewClient(nil), max, cfg.Power.ConcurrencyBattery)
	}
	return &Queue{DB: database, Registry: reg, Fetcher: dl, Cfg: cfg, running: map[int64]bool{}, batteryPct: -1}
}

func (q *Queue) SetBattery(pct int, charging bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.batteryPct = pct
	q.charging = charging
}

func (q *Queue) CheckBattery() (bool, string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.charging {
		if q.batteryPct < 0 {
			return false, "battery reading unavailable"
		}
		if q.batteryPct <= q.Cfg.Power.MinBatteryPct {
			return false, fmt.Sprintf("battery %d%% at or below minimum", q.batteryPct)
		}
	}
	return true, ""
}

func (q *Queue) Pump(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	q.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			q.tick(ctx)
		}
	}
}

func (q *Queue) tick(ctx context.Context) {
	ok, _ := q.CheckBattery()
	if !ok {
		return
	}
	q.mu.Lock()
	maxConcurrency := q.Cfg.Power.ConcurrencyBattery
	if q.charging {
		maxConcurrency = q.Cfg.Power.ConcurrencyCharging
	}
	active := 0
	for _, running := range q.running {
		if running {
			active++
		}
	}
	q.mu.Unlock()
	if maxConcurrency <= 0 {
		maxConcurrency = 2
	}
	if active >= maxConcurrency {
		return
	}
	jobs, err := q.DB.ListJobs(ctx, []string{string(hermit.JobQueued)}, maxConcurrency-active)
	if err != nil || len(jobs) == 0 {
		return
	}
	for _, j := range jobs {
		if active >= maxConcurrency {
			break
		}
		q.mu.Lock()
		if q.running[j.ID] {
			q.mu.Unlock()
			continue
		}
		q.running[j.ID] = true
		q.mu.Unlock()
		active++
		go q.runJob(ctx, j)
	}
}

func (q *Queue) runJob(ctx context.Context, job hermit.Job) {
	defer func() {
		q.mu.Lock()
		delete(q.running, job.ID)
		q.mu.Unlock()
	}()
	tmpDir := filepath.Join(q.Cfg.StateDir, "tmp", fmt.Sprintf("job-%d", job.ID))
	defer os.RemoveAll(tmpDir)

	reserve, _ := config.ParseSize(q.Cfg.Disk.Reserve)
	margin, _ := config.ParseSize(q.Cfg.Disk.Margin)
	job.State = hermit.JobDownloading
	job.StartedAt = time.Now()
	if err := q.DB.UpdateJob(ctx, job); err != nil {
		q.failJob(ctx, job, "database", err.Error())
		return
	}

	if job.Source.URL == "" {
		show := q.showTitle(ctx, job)
		ref := hermit.Ref{Provider: job.Provider, Season: job.Season, Episode: job.Episode, Title: show, Kind: hermit.KindTV}
		srcs, errs := q.Registry.ResolveAll(ctx, ref)
		if len(srcs) == 0 {
			q.failJob(ctx, job, "resolve", firstErr(errs).Error())
			return
		}
		job.Source = srcs[0]
	}

	fits, _ := disk.Fits(q.Cfg.Library.Path, job.Source.SizeBytes, reserve, margin)
	if job.Source.SizeBytes > 0 && !fits {
		q.failJob(ctx, job, "disk_headroom", "insufficient spare space for job")
		return
	}

	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		q.failJob(ctx, job, "storage_perm_lost", err.Error())
		return
	}
	tmpPath := filepath.Join(tmpDir, "download.part")
	job.TmpPath = tmpPath
	if err := q.DB.UpdateJob(ctx, job); err != nil {
		q.failJob(ctx, job, "database", err.Error())
		return
	}

	res, err := q.Fetcher.Download(ctx, job.Source, tmpPath)
	if err != nil {
		q.failJob(ctx, job, "truncated", err.Error())
		return
	}

	show := q.showTitle(ctx, job)
	epTitle := q.episodeTitle(ctx, job)
	targetName := filepath.Base(job.TargetPath)
	if targetName == "" || targetName == "." || targetName == string(filepath.Separator) {
		targetName = fmt.Sprintf("%s - S%sE%s - %s.mp4", scrub.SafeName(show), two(job.Season), two(job.Episode), scrub.SafeName(epTitle))
		targetName = scrub.LimitPathComponent(targetName)
	}
	target := filepath.Join(q.Cfg.Library.Path, scrub.SafeName(show), targetName)
	job.TargetPath = target
	job.SHA256 = res.SHA256
	job.BytesTotal = res.Bytes
	job.BytesDone = res.Bytes
	job.PartsDone = res.Parts

	if q.Cfg.Power.DeferRemuxToCharge {
		if err := renameAtomic(tmpPath, target); err != nil {
			q.failJob(ctx, job, "storage_perm_lost", err.Error())
			return
		}
		if err := scrub.WriteSidecar(target, job.MediaID, job.SHA256); err != nil {
			q.failJob(ctx, job, "storage_metadata", err.Error())
			return
		}
		job.State = hermit.JobDone
		q.finishJob(ctx, job)
		return
	}

	remuxed := false
	if mux.Available() {
		job.State = hermit.JobRemuxing
		if err := q.DB.UpdateJob(ctx, job); err != nil {
			q.failJob(ctx, job, "database", err.Error())
			return
		}
		tmp2 := target + ".tmp"
		r, remuxErr := mux.Remux(tmpPath, tmp2, scrub.Line(show, job.Season, job.Episode, epTitle), scrub.SafeName(show), "", "", true)
		if remuxErr == nil && r {
			if err := renameAtomic(tmp2, target); err != nil {
				_ = os.Remove(tmp2)
				q.failJob(ctx, job, "storage_perm_lost", err.Error())
				return
			}
			remuxed = true
			_ = os.Remove(tmpPath)
		} else {
			_ = os.Remove(tmp2)
		}
	}
	if !remuxed {
		if err := renameAtomic(tmpPath, target); err != nil {
			q.failJob(ctx, job, "storage_perm_lost", err.Error())
			return
		}
	}
	job.State = hermit.JobDone
	if err := q.DB.UpdateJob(ctx, job); err != nil {
		return
	}
	if err := scrub.WriteSidecar(target, job.MediaID, job.SHA256); err != nil {
		job.LastError = err.Error()
		job.ErrKind = "storage_metadata"
		_ = q.DB.UpdateJob(ctx, job)
		return
	}
	q.finishJob(ctx, job)
}

func (q *Queue) showTitle(ctx context.Context, job hermit.Job) string {
	mediaList, err := q.DB.ListMedia(ctx, 500, 0)
	if err != nil {
		return fmt.Sprintf("Media %d", job.MediaID)
	}
	for _, m := range mediaList {
		if m.ID == job.MediaID {
			return m.Title
		}
	}
	return fmt.Sprintf("Media %d", job.MediaID)
}

func (q *Queue) episodeTitle(ctx context.Context, job hermit.Job) string {
	episodes, err := q.DB.GetEpisodes(ctx, job.MediaID, job.Season)
	if err != nil {
		return fmt.Sprintf("Episode %d", job.Episode)
	}
	for _, e := range episodes {
		if e.Episode == job.Episode {
			return e.Title
		}
	}
	return fmt.Sprintf("Episode %d", job.Episode)
}

func firstErr(errs map[string]error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return fmt.Errorf("all providers failed")
}

func (q *Queue) failJob(ctx context.Context, job hermit.Job, kind, msg string) {
	job.LastError = msg
	job.ErrKind = kind
	job.TmpPath = ""
	if q.Cfg.Power.MaxAttempts > 0 && job.Attempts < q.Cfg.Power.MaxAttempts {
		job.Attempts++
		job.State = hermit.JobQueued
		job.NextRetryAt = time.Now().Add(retryBackoff(job.Attempts))
		job.FinishedAt = time.Time{}
	} else {
		job.State = hermit.JobFailed
		job.FinishedAt = time.Now()
		job.NextRetryAt = time.Time{}
	}
	_ = q.DB.UpdateJob(ctx, job)
}

func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	backoff := 30 * time.Second
	for i := 1; i < attempt; i++ {
		backoff *= 2
		if backoff >= 30*time.Minute {
			return 30 * time.Minute
		}
	}
	return backoff
}

func (q *Queue) finishJob(ctx context.Context, job hermit.Job) {
	job.State = hermit.JobDone
	job.FinishedAt = time.Now()
	_ = q.DB.UpdateJob(ctx, job)
}

func renameAtomic(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.Rename(src, dst)
}

func two(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func itoa(n int) string {
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
