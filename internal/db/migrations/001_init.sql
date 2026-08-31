CREATE TABLE media (
    id INTEGER PRIMARY KEY,
    kind TEXT NOT NULL,
    tmdb_id INTEGER UNIQUE,
    imdb_id TEXT,
    anilist_id INTEGER UNIQUE,
    slug TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    original_title TEXT NOT NULL DEFAULT '',
    poster_path TEXT NOT NULL DEFAULT '',
    backdrop_path TEXT NOT NULL DEFAULT '',
    overview TEXT NOT NULL DEFAULT '',
    first_air TEXT NOT NULL DEFAULT '',
    runtime_min INTEGER NOT NULL DEFAULT 0,
    season_count INTEGER NOT NULL DEFAULT 0,
    episode_count INTEGER NOT NULL DEFAULT 0,
    year INTEGER NOT NULL DEFAULT 0,
    raw TEXT NOT NULL DEFAULT '',
    meta_fetched_at TEXT NOT NULL DEFAULT '',
    meta_ttl REAL NOT NULL DEFAULT 0
);

CREATE TABLE season (
    id INTEGER PRIMARY KEY,
    media_id INTEGER NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    season INTEGER NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    air_year INTEGER NOT NULL DEFAULT 0,
    episode_count INTEGER NOT NULL DEFAULT 0,
    availability_json TEXT NOT NULL DEFAULT '',
    UNIQUE(media_id, season)
);

CREATE TABLE episode (
    id INTEGER PRIMARY KEY,
    media_id INTEGER NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    season INTEGER NOT NULL,
    episode INTEGER NOT NULL,
    tmdb_ep_id INTEGER,
    title TEXT NOT NULL DEFAULT '',
    air_date TEXT NOT NULL DEFAULT '',
    runtime_s INTEGER NOT NULL DEFAULT 0,
    still_path TEXT NOT NULL DEFAULT '',
    UNIQUE(media_id, season, episode)
);

CREATE TABLE job (
    id INTEGER PRIMARY KEY,
    episode_id INTEGER,
    media_id INTEGER,
    season INTEGER NOT NULL DEFAULT 0,
    episode INTEGER NOT NULL DEFAULT 0,
    provider TEXT NOT NULL,
    source_json TEXT NOT NULL DEFAULT '',
    quality TEXT NOT NULL DEFAULT 'auto',
    codec TEXT NOT NULL DEFAULT 'auto',
    state TEXT NOT NULL DEFAULT 'queued',
    priority INTEGER NOT NULL DEFAULT 100,
    bytes_total INTEGER NOT NULL DEFAULT 0,
    bytes_done INTEGER NOT NULL DEFAULT 0,
    parts_total INTEGER NOT NULL DEFAULT 0,
    parts_done INTEGER NOT NULL DEFAULT 0,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    err_kind TEXT NOT NULL DEFAULT '',
    sha256 TEXT NOT NULL DEFAULT '',
    target_path TEXT NOT NULL DEFAULT '',
    tmp_path TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    started_at TEXT NOT NULL DEFAULT '',
    finished_at TEXT NOT NULL DEFAULT '',
    next_retry_at TEXT NOT NULL DEFAULT '',
    UNIQUE(episode_id, provider, quality)
);

CREATE TABLE job_part (
    job_id INTEGER NOT NULL REFERENCES job(id) ON DELETE CASCADE,
    idx INTEGER NOT NULL,
    url TEXT NOT NULL DEFAULT '',
    bytes_expected INTEGER NOT NULL DEFAULT 0,
    bytes_done INTEGER NOT NULL DEFAULT 0,
    state TEXT NOT NULL DEFAULT '',
    PRIMARY KEY(job_id, idx)
);

CREATE TABLE availability (
    provider TEXT NOT NULL,
    episode_id INTEGER NOT NULL,
    ok INTEGER NOT NULL DEFAULT 0,
    quality TEXT NOT NULL DEFAULT '',
    codec TEXT NOT NULL DEFAULT '',
    size_bytes INTEGER NOT NULL DEFAULT 0,
    score REAL NOT NULL DEFAULT 0,
    checked_at TEXT NOT NULL DEFAULT '',
    ttl REAL NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '',
    PRIMARY KEY(provider, episode_id)
);

CREATE TABLE host_stats (
    origin TEXT PRIMARY KEY,
    ewma_success REAL NOT NULL DEFAULT 0.7,
    samples INTEGER NOT NULL DEFAULT 0,
    median_latency_ms INTEGER NOT NULL DEFAULT 0,
    last_ok_at TEXT NOT NULL DEFAULT '',
    last_fail_at TEXT NOT NULL DEFAULT '',
    banned_until TEXT NOT NULL DEFAULT ''
);

CREATE TABLE playback (
    episode_id INTEGER PRIMARY KEY,
    position_s REAL NOT NULL DEFAULT 0,
    duration_s REAL NOT NULL DEFAULT 0,
    completed INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT ''
);

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value_json TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_job_state ON job(state, priority, next_retry_at);
CREATE INDEX idx_episode_media ON episode(media_id, season, episode);
CREATE INDEX idx_availability_episode ON availability(episode_id);
CREATE INDEX idx_playback_completed ON playback(completed);
CREATE INDEX idx_media_title ON media(title);
