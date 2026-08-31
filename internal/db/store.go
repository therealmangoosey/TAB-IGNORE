package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

const timeLayout = time.RFC3339Nano

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(timeLayout)
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// SaveMedia inserts or updates a media row and returns its stable ID.
func (d *DB) SaveMedia(ctx context.Context, m hermit.Media) (int64, error) {
	ttl := float64(24 * 3600)
	if !m.MetaFetchedAt.IsZero() {
		ttl = 24 * 3600
	}
	var id int64
	err := d.conn.QueryRowContext(ctx,
		`INSERT INTO media(kind, tmdb_id, imdb_id, anilist_id, slug, title, original_title, poster_path, backdrop_path, overview, first_air, runtime_min, season_count, episode_count, year, raw, meta_fetched_at, meta_ttl)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(tmdb_id) DO UPDATE SET
		   kind=excluded.kind, imdb_id=excluded.imdb_id, anilist_id=excluded.anilist_id,
		   slug=excluded.slug, title=excluded.title, original_title=excluded.original_title,
		   poster_path=excluded.poster_path, backdrop_path=excluded.backdrop_path,
		   overview=excluded.overview, first_air=excluded.first_air, runtime_min=excluded.runtime_min,
		   season_count=excluded.season_count, episode_count=excluded.episode_count, year=excluded.year,
		   raw=excluded.raw, meta_fetched_at=excluded.meta_fetched_at, meta_ttl=excluded.meta_ttl
		 RETURNING id`,
		m.Kind, nullableInt(m.TMDBID), nullableStr(m.IMDBID), nullableInt(m.AniListID), m.Slug, m.Title, m.OriginalTitle,
		m.PosterPath, m.BackdropPath, m.Overview, m.FirstAir, m.RuntimeMin, m.SeasonCount, m.EpisodeCount, m.Year,
		m.Raw, formatTime(m.MetaFetchedAt), ttl,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert media: %w", err)
	}
	return id, nil
}

func nullableInt(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func now() time.Time { return time.Now() }

// GetMediaByTMDB returns a cached media row.
func (d *DB) GetMediaByTMDB(ctx context.Context, tmdb int, kind hermit.Kind) (hermit.Media, error) {
	var m hermit.Media
	var idStr, anilist sql.NullString
	var fetched string
	err := d.conn.QueryRowContext(ctx,
		`SELECT id, kind, tmdb_id, imdb_id, anilist_id, slug, title, original_title, poster_path, backdrop_path, overview, first_air, runtime_min, season_count, episode_count, year, raw, meta_fetched_at FROM media WHERE tmdb_id=? AND (kind=? OR kind='movie' OR kind='tv') LIMIT 1`,
		tmdb, kind,
	).Scan(&m.ID, &m.Kind, &m.TMDBID, &idStr, &anilist, &m.Slug, &m.Title, &m.OriginalTitle, &m.PosterPath, &m.BackdropPath,
		&m.Overview, &m.FirstAir, &m.RuntimeMin, &m.SeasonCount, &m.EpisodeCount, &m.Year, &m.Raw, &fetched)
	if err != nil {
		return m, err
	}
	m.IMDBID = idStr.String
	if anilist.Valid {
		fmt.Sscanf(anilist.String, "%d", &m.AniListID)
	}
	m.MetaFetchedAt = parseTime(fetched)
	return m, nil
}

// ListMedia returns a paged media list ordered by title.
func (d *DB) ListMedia(ctx context.Context, limit, offset int) ([]hermit.Media, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, kind, tmdb_id, imdb_id, anilist_id, slug, title, original_title, poster_path, backdrop_path, overview, first_air, runtime_min, season_count, episode_count, year, raw, meta_fetched_at FROM media ORDER BY title COLLATE NOCASE LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []hermit.Media
	for rows.Next() {
		var m hermit.Media
		var idStr, anilist sql.NullString
		var fetched string
		if err := rows.Scan(&m.ID, &m.Kind, &m.TMDBID, &idStr, &anilist, &m.Slug, &m.Title, &m.OriginalTitle, &m.PosterPath,
			&m.BackdropPath, &m.Overview, &m.FirstAir, &m.RuntimeMin, &m.SeasonCount, &m.EpisodeCount, &m.Year, &m.Raw,
			&fetched); err != nil {
			return nil, err
		}
		m.IMDBID = idStr.String
		if anilist.Valid {
			fmt.Sscanf(anilist.String, "%d", &m.AniListID)
		}
		m.MetaFetchedAt = parseTime(fetched)
		out = append(out, m)
	}
	return out, rows.Err()
}

// SaveSeason inserts or updates a season row.
func (d *DB) SaveSeason(ctx context.Context, s hermit.Season) error {
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO season(media_id, season, name, air_year, episode_count, availability_json)
		 VALUES (?,?,?,?,?,?)
		 ON CONFLICT(media_id, season) DO UPDATE SET
		   name=excluded.name, air_year=excluded.air_year, episode_count=excluded.episode_count,
		   availability_json=excluded.availability_json`,
		s.MediaID, s.Season, s.Name, s.AirYear, s.EpisodeCount, s.AvailabilityJSON())
	return err
}

// GetSeasons returns seasons for a media row.
func (d *DB) GetSeasons(ctx context.Context, mediaID int64) ([]hermit.Season, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, media_id, season, name, air_year, episode_count, availability_json FROM season WHERE media_id=? ORDER BY season`, mediaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []hermit.Season
	for rows.Next() {
		var s hermit.Season
		var avail string
		if err := rows.Scan(&s.ID, &s.MediaID, &s.Season, &s.Name, &s.AirYear, &s.EpisodeCount, &avail); err != nil {
			return nil, err
		}
		s.SetAvailabilityJSON(avail)
		out = append(out, s)
	}
	return out, rows.Err()
}

// SaveEpisode inserts or updates an episode row and returns its ID.
func (d *DB) SaveEpisode(ctx context.Context, e hermit.Episode) (int64, error) {
	var id int64
	err := d.conn.QueryRowContext(ctx,
		`INSERT INTO episode(media_id, season, episode, tmdb_ep_id, title, air_date, runtime_s, still_path)
		 VALUES (?,?,?,?,?,?,?,?)
		 ON CONFLICT(media_id, season, episode) DO UPDATE SET
		   tmdb_ep_id=excluded.tmdb_ep_id, title=excluded.title, air_date=excluded.air_date,
		   runtime_s=excluded.runtime_s, still_path=excluded.still_path
		 RETURNING id`,
		e.MediaID, e.Season, e.Episode, nullableInt(e.TMDBEpID), e.Title, e.AirDate, e.RuntimeS, e.StillPath,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// GetEpisodes returns episodes for a media/season, optionally limited.
func (d *DB) GetEpisodes(ctx context.Context, mediaID int64, season int) ([]hermit.Episode, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, media_id, season, episode, tmdb_ep_id, title, air_date, runtime_s, still_path
		 FROM episode WHERE media_id=? AND (?=0 OR season=?) ORDER BY season, episode`,
		mediaID, season, season)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []hermit.Episode
	for rows.Next() {
		var e hermit.Episode
		var epID sql.NullInt64
		if err := rows.Scan(&e.ID, &e.MediaID, &e.Season, &e.Episode, &epID, &e.Title, &e.AirDate, &e.RuntimeS, &e.StillPath); err != nil {
			return nil, err
		}
		if epID.Valid {
			e.TMDBEpID = int(epID.Int64)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// InsertJob inserts a queued job and returns its ID.
func (d *DB) InsertJob(ctx context.Context, j hermit.Job) (int64, error) {
	src, _ := json.Marshal(j.Source)
	var id int64
	err := d.conn.QueryRowContext(ctx,
		`INSERT INTO job(episode_id, media_id, season, episode, provider, source_json, quality, codec, state, priority,
		                bytes_total, bytes_done, parts_total, parts_done, attempts, last_error, err_kind, sha256,
		                target_path, tmp_path, created_at, started_at, finished_at, next_retry_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(episode_id, provider, quality) DO UPDATE SET
		   state='queued', priority=excluded.priority, bytes_total=excluded.bytes_total, bytes_done=0,
		   parts_total=excluded.parts_total, parts_done=0, attempts=0, last_error='', err_kind='',
		   source_json=excluded.source_json, tmp_path=excluded.tmp_path, target_path=excluded.target_path
		 RETURNING id`,
		nullableInt64(j.EpisodeID), nullableInt64(j.MediaID), j.Season, j.Episode, j.Provider, string(src),
		string(j.Quality), string(j.Codec), string(j.State), j.Priority, j.BytesTotal, j.BytesDone,
		j.PartsTotal, j.PartsDone, j.Attempts, j.LastError, j.ErrKind, j.SHA256, j.TargetPath, j.TmpPath,
		formatTime(j.CreatedAt), formatTime(j.StartedAt), formatTime(j.FinishedAt), formatTime(j.NextRetryAt),
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func nullableInt64(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}

// UpdateJob persists a job's mutable fields.
func (d *DB) UpdateJob(ctx context.Context, j hermit.Job) error {
	src, _ := json.Marshal(j.Source)
	_, err := d.conn.ExecContext(ctx,
		`UPDATE job SET season=?, episode=?, provider=?, source_json=?, quality=?, codec=?, state=?, priority=?,
		   bytes_total=?, bytes_done=?, parts_total=?, parts_done=?, attempts=?, last_error=?, err_kind=?,
		   sha256=?, target_path=?, tmp_path=?, created_at=?, started_at=?, finished_at=?, next_retry_at=? WHERE id=?`,
		j.Season, j.Episode, j.Provider, string(src), string(j.Quality), string(j.Codec), string(j.State), j.Priority,
		j.BytesTotal, j.BytesDone, j.PartsTotal, j.PartsDone, j.Attempts, j.LastError, j.ErrKind, j.SHA256,
		j.TargetPath, j.TmpPath, formatTime(j.CreatedAt), formatTime(j.StartedAt), formatTime(j.FinishedAt),
		formatTime(j.NextRetryAt), j.ID)
	return err
}

// GetJob returns a job by ID.
func (d *DB) GetJob(ctx context.Context, id int64) (hermit.Job, error) {
	var j hermit.Job
	var src string
	var createdAt, startedAt, finishedAt, nextRetryAt string
	err := d.conn.QueryRowContext(ctx,
		`SELECT id, COALESCE(episode_id,0), COALESCE(media_id,0), season, episode, provider, source_json, quality, codec, state,
		        priority, bytes_total, bytes_done, parts_total, parts_done, attempts, last_error, err_kind, sha256,
		        target_path, tmp_path, created_at, started_at, finished_at, next_retry_at FROM job WHERE id=?`,
		id).Scan(&j.ID, &j.EpisodeID, &j.MediaID, &j.Season, &j.Episode, &j.Provider, &src, &j.Quality, &j.Codec,
		&j.State, &j.Priority, &j.BytesTotal, &j.BytesDone, &j.PartsTotal, &j.PartsDone, &j.Attempts, &j.LastError,
		&j.ErrKind, &j.SHA256, &j.TargetPath, &j.TmpPath, &createdAt, &startedAt, &finishedAt, &nextRetryAt)
	if err != nil {
		return j, err
	}
	j.CreatedAt = parseTime(createdAt)
	j.StartedAt = parseTime(startedAt)
	j.FinishedAt = parseTime(finishedAt)
	j.NextRetryAt = parseTime(nextRetryAt)
	if err := json.Unmarshal([]byte(src), &j.Source); err != nil {
		j.Source = hermit.Source{}
	}
	return j, nil
}

// ListJobs returns jobs filtered by state list.
func (d *DB) ListJobs(ctx context.Context, states []string, limit int) ([]hermit.Job, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	where := ""
	args := []any{}
	if len(states) > 0 {
		ph := make([]string, len(states))
		for i, s := range states {
			ph[i] = "?"
			args = append(args, s)
		}
		where = " WHERE state IN (" + strings.Join(ph, ",") + ")"
	}
	args = append(args, limit)
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, COALESCE(episode_id,0), COALESCE(media_id,0), season, episode, provider, source_json, quality, codec, state,
		        priority, bytes_total, bytes_done, parts_total, parts_done, attempts, last_error, err_kind, sha256,
		        target_path, tmp_path, created_at, started_at, finished_at, next_retry_at
		 FROM job`+where+` ORDER BY priority, created_at LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []hermit.Job
	for rows.Next() {
		var j hermit.Job
		var src string
		var createdAt, startedAt, finishedAt, nextRetryAt string
		if err := rows.Scan(&j.ID, &j.EpisodeID, &j.MediaID, &j.Season, &j.Episode, &j.Provider, &src, &j.Quality,
			&j.Codec, &j.State, &j.Priority, &j.BytesTotal, &j.BytesDone, &j.PartsTotal, &j.PartsDone, &j.Attempts,
			&j.LastError, &j.ErrKind, &j.SHA256, &j.TargetPath, &j.TmpPath, &createdAt, &startedAt, &finishedAt,
			&nextRetryAt); err != nil {
			return nil, err
		}
		j.CreatedAt = parseTime(createdAt)
		j.StartedAt = parseTime(startedAt)
		j.FinishedAt = parseTime(finishedAt)
		j.NextRetryAt = parseTime(nextRetryAt)
		_ = json.Unmarshal([]byte(src), &j.Source)
		out = append(out, j)
	}
	return out, rows.Err()
}

// SaveJobParts replaces all parts for a job.
func (d *DB) SaveJobParts(ctx context.Context, jobID int64, parts []hermit.JobPart) error {
	if _, err := d.conn.ExecContext(ctx, `DELETE FROM job_part WHERE job_id=?`, jobID); err != nil {
		return err
	}
	for _, p := range parts {
		if _, err := d.conn.ExecContext(ctx,
			`INSERT INTO job_part(job_id, idx, url, bytes_expected, bytes_done, state) VALUES (?,?,?,?,?,?)`,
			jobID, p.Index, p.URL, p.BytesExpected, p.BytesDone, p.State); err != nil {
			return err
		}
	}
	return nil
}

// GetJobParts returns the parts for a job.
func (d *DB) GetJobParts(ctx context.Context, jobID int64) ([]hermit.JobPart, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT job_id, idx, url, bytes_expected, bytes_done, state FROM job_part WHERE job_id=? ORDER BY idx`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []hermit.JobPart
	for rows.Next() {
		var p hermit.JobPart
		if err := rows.Scan(&p.JobID, &p.Index, &p.URL, &p.BytesExpected, &p.BytesDone, &p.State); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SavePlayback upserts playback state.
func (d *DB) SavePlayback(ctx context.Context, p hermit.Playback) error {
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO playback(episode_id, position_s, duration_s, completed, updated_at, source)
		 VALUES (?,?,?,?,?,?)
		 ON CONFLICT(episode_id) DO UPDATE SET
		   position_s=excluded.position_s, duration_s=excluded.duration_s, completed=excluded.completed,
		   updated_at=excluded.updated_at, source=excluded.source`,
		p.EpisodeID, p.PositionS, p.DurationS, boolInt(p.Completed), formatTime(p.UpdatedAt), p.Source)
	return err
}

// GetPlayback returns playback state, or an error if none exists.
func (d *DB) GetPlayback(ctx context.Context, episodeID int64) (hermit.Playback, error) {
	var p hermit.Playback
	var completed int
	var updatedAt string
	err := d.conn.QueryRowContext(ctx,
		`SELECT episode_id, position_s, duration_s, completed, updated_at, source FROM playback WHERE episode_id=?`, episodeID).
		Scan(&p.EpisodeID, &p.PositionS, &p.DurationS, &completed, &updatedAt, &p.Source)
	p.UpdatedAt = parseTime(updatedAt)
	p.Completed = completed != 0
	return p, err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
