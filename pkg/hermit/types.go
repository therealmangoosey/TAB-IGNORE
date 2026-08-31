// Package hermit defines the public data model used by the CLI, the daemon
// JSON-RPC interface, the TUI, and external clients.
package hermit

import "time"

// Kind is the broad media kind.
type Kind string

const (
	KindMovie Kind = "movie"
	KindTV    Kind = "tv"
	KindAnime Kind = "anime"
)

// Quality is a display quality value.
type Quality string

const (
	QualityAuto Quality = "auto"
	Quality720  Quality = "720p"
	Quality1080 Quality = "1080p"
)

// Codec is a video codec preference.
type Codec string

const (
	CodecAuto    Codec = "auto"
	CodecHEVC    Codec = "hevc"
	CodecAVC     Codec = "avc"
	CodecAV1     Codec = "av1"
	CodecUnknown Codec = "unknown"
)

// TransportKind is how a source is delivered.
type TransportKind string

const (
	TransportDirect TransportKind = "direct"
	TransportHLS    TransportKind = "hls"
	TransportDASH   TransportKind = "dash"
)

// Media is a show or movie stored in the metadata cache.
type Media struct {
	ID            int64     `json:"id"`
	Kind          Kind      `json:"kind"`
	TMDBID        int       `json:"tmdb_id,omitempty"`
	IMDBID        string    `json:"imdb_id,omitempty"`
	AniListID     int       `json:"anilist_id,omitempty"`
	Slug          string    `json:"slug"`
	Title         string    `json:"title"`
	OriginalTitle string    `json:"original_title,omitempty"`
	PosterPath    string    `json:"poster_path,omitempty"`
	BackdropPath  string    `json:"backdrop_path,omitempty"`
	Overview      string    `json:"overview,omitempty"`
	FirstAir      string    `json:"first_air,omitempty"`
	RuntimeMin    int       `json:"runtime_min,omitempty"`
	SeasonCount   int       `json:"season_count,omitempty"`
	EpisodeCount  int       `json:"episode_count,omitempty"`
	Year          int       `json:"year,omitempty"`
	MetaFetchedAt time.Time `json:"meta_fetched_at,omitempty"`
	Raw           string    `json:"raw,omitempty"`
}

// Season is a season belonging to a Media.
type Season struct {
	ID           int64  `json:"id"`
	MediaID      int64  `json:"media_id"`
	Season       int    `json:"season"`
	Name         string `json:"name"`
	AirYear      int    `json:"air_year,omitempty"`
	EpisodeCount int    `json:"episode_count"`
	Available    int    `json:"available"`
	Partial      bool   `json:"partial"`
	AvailJSON    string `json:"-"`
}

// AvailabilityJSON returns the stored availability cache as JSON.
func (s Season) AvailabilityJSON() string {
	return s.AvailJSON
}

// SetAvailabilityJSON stores an availability cache blob.
func (s *Season) SetAvailabilityJSON(raw string) {
	s.AvailJSON = raw
}

// Episode is one episode of a season.
type Episode struct {
	ID           int64  `json:"id"`
	MediaID      int64  `json:"media_id"`
	Season       int    `json:"season"`
	Episode      int    `json:"episode"`
	TMDBEpID     int    `json:"tmdb_ep_id,omitempty"`
	Title        string `json:"title"`
	AirDate      string `json:"air_date,omitempty"`
	RuntimeS     int    `json:"runtime_s,omitempty"`
	StillPath    string `json:"still_path,omitempty"`
	Available    bool   `json:"available"`
}

// Source is one candidate media source returned by a provider.
type Source struct {
	ID          string         `json:"id"`
	Label       string         `json:"label"`
	Provider    string         `json:"provider"`
	Kind        TransportKind  `json:"kind"`
	URL         string         `json:"url,omitempty"`
	Referer     string         `json:"referer,omitempty"`
	Quality     Quality        `json:"quality"`
	Codec       Codec          `json:"codec"`
	SizeBytes   int64          `json:"size_bytes"`
	HasSubtitles bool          `json:"has_subtitles"`
	Languages   []string       `json:"languages,omitempty"`
	Score       float64        `json:"score"`
	LatencyMS   int64          `json:"latency_ms,omitempty"`
	ExpiresAt   time.Time      `json:"expires_at,omitempty"`
	Raw         string         `json:"raw,omitempty"`
}

// Hit is a search result.
type Hit struct {
	Ref      Ref     `json:"ref"`
	Title    string  `json:"title"`
	Year     int     `json:"year,omitempty"`
	Provider string  `json:"provider"`
	Kind     Kind    `json:"kind"`
	Score    float64 `json:"score,omitempty"`
}

// Ref identifies media using external IDs and the provider.
type Ref struct {
	Kind      Kind   `json:"kind"`
	TMDBID    int    `json:"tmdb_id,omitempty"`
	IMDBID    string `json:"imdb_id,omitempty"`
	AniListID int    `json:"anilist_id,omitempty"`
	Season    int    `json:"season,omitempty"`
	Episode   int    `json:"episode,omitempty"`
	Title     string `json:"title,omitempty"`
	Year      int    `json:"year,omitempty"`
	Provider  string `json:"provider,omitempty"`
}

// JobState is the persisted download state.
type JobState string

const (
	JobQueued      JobState = "queued"
	JobResolving   JobState = "resolving"
	JobProbing     JobState = "probing"
	JobDownloading JobState = "downloading"
	JobVerifying   JobState = "verifying"
	JobRemuxing    JobState = "remuxing"
	JobDone        JobState = "done"
	JobFailed      JobState = "failed"
	JobPaused      JobState = "paused"
	JobCanceled    JobState = "canceled"
)

// Job is a download job.
type Job struct {
	ID          int64     `json:"id"`
	EpisodeID   int64     `json:"episode_id,omitempty"`
	MediaID     int64     `json:"media_id,omitempty"`
	Season      int       `json:"season"`
	Episode     int       `json:"episode"`
	Provider    string    `json:"provider"`
	Source      Source    `json:"source"`
	Quality     Quality   `json:"quality"`
	Codec       Codec     `json:"codec"`
	State       JobState  `json:"state"`
	Priority    int       `json:"priority"`
	BytesTotal  int64     `json:"bytes_total"`
	BytesDone   int64     `json:"bytes_done"`
	PartsTotal  int       `json:"parts_total"`
	PartsDone   int       `json:"parts_done"`
	Attempts    int       `json:"attempts"`
	LastError   string    `json:"last_error,omitempty"`
	ErrKind     string    `json:"err_kind,omitempty"`
	SHA256      string    `json:"sha256,omitempty"`
	TargetPath  string    `json:"target_path,omitempty"`
	TmpPath     string    `json:"tmp_path,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	FinishedAt  time.Time `json:"finished_at,omitempty"`
	NextRetryAt time.Time `json:"next_retry_at,omitempty"`
}

// JobPart is one resumable piece of a download.
type JobPart struct {
	JobID        int64  `json:"job_id"`
	Index        int    `json:"index"`
	URL          string `json:"url"`
	BytesExpected int64 `json:"bytes_expected"`
	BytesDone    int64  `json:"bytes_done"`
	State        string `json:"state"`
}

// Playback is saved playback position.
type Playback struct {
	EpisodeID int64     `json:"episode_id"`
	PositionS float64   `json:"position_s"`
	DurationS float64   `json:"duration_s"`
	Completed bool      `json:"completed"`
	UpdatedAt time.Time `json:"updated_at"`
	Source    string    `json:"source"`
}

// ProviderHealth is a doctor/status line for one provider.
type ProviderHealth struct {
	ID       string    `json:"id"`
	Enabled  bool      `json:"enabled"`
	OK       bool      `json:"ok"`
	Base     string    `json:"base"`
	LastErr  string    `json:"last_error,omitempty"`
	LatencyMS int64    `json:"latency_ms,omitempty"`
	CheckedAt time.Time `json:"checked_at,omitempty"`
}

// DeviceProfile describes the runtime device.
type DeviceProfile struct {
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	GoVersion   string `json:"go_version"`
	CPUCount    int    `json:"cpu_count"`
	MemoryBytes uint64 `json:"memory_bytes"`
	Termux      bool   `json:"termux"`
	BatteryPct  int    `json:"battery_pct"`
	Charging    bool   `json:"charging"`
	FFmpeg      bool   `json:"ffmpeg"`
	Mpv         bool   `json:"mpv"`
	VLC         bool   `json:"vlc"`
	StorageOK   bool   `json:"storage_ok"`
	StorageMode string `json:"storage_mode"`
	PrivacyOK   bool   `json:"privacy_ok"`
	ProfileName string `json:"profile"`
}

// Status is the daemon status snapshot.
type Status struct {
	Version    string            `json:"version"`
	Profile    DeviceProfile     `json:"profile"`
	Jobs       []Job             `json:"jobs,omitempty"`
	Providers  []ProviderHealth  `json:"providers,omitempty"`
	LibrarySize int64            `json:"library_size"`
	LibraryFiles int             `json:"library_files"`
	SpareBytes int64             `json:"spare_bytes"`
	FreeBytes  int64             `json:"free_bytes"`
	ReserveBytes int64           `json:"reserve_bytes"`
	Uptime     string            `json:"uptime,omitempty"`
	ServerAddr string            `json:"server_addr"`
}

// RPCError is the JSON-RPC error envelope.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
