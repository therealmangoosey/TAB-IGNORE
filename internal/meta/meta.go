// Package meta talks to documented metadata providers (TMDB and AniList) and
// caches results in SQLite. It never uses a sketchy aggregator for metadata.
package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/internal/db"
	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

// Client is a metadata provider client.
type Client struct {
	DB       *db.DB
	HTTP     *http.Client
	TMDBKey  string
	Base     string
	CacheTTL time.Duration
}

// NewClient creates a metadata client with the default 24-hour cache TTL.
func NewClient(database *db.DB, httpClient *http.Client, tmdbKey string) *Client {
	return NewClientWithTTL(database, httpClient, tmdbKey, 24*time.Hour)
}

// NewClientWithTTL creates a metadata client with an explicit cache TTL.
func NewClientWithTTL(database *db.DB, httpClient *http.Client, tmdbKey string, cacheTTL time.Duration) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	if database == nil {
		panic("meta client requires a database")
	}
	if cacheTTL <= 0 {
		cacheTTL = 24 * time.Hour
	}
	return &Client{DB: database, HTTP: httpClient, TMDBKey: tmdbKey, Base: "https://api.themoviedb.org/3", CacheTTL: cacheTTL}
}

type tmdbSearchResp struct {
	Results []struct {
		ID              int     `json:"id"`
		Name            string  `json:"name"`
		Title           string  `json:"title"`
		MediaType       string  `json:"media_type"`
		OriginalName    string  `json:"original_name"`
		OriginalTitle   string  `json:"original_title"`
		Overview        string  `json:"overview"`
		PosterPath      string  `json:"poster_path"`
		BackdropPath    string  `json:"backdrop_path"`
		FirstAirDate    string  `json:"first_air_date"`
		ReleaseDate     string  `json:"release_date"`
		VoteAverage     float64 `json:"vote_average"`
		Popularity      float64 `json:"popularity"`
	} `json:"results"`
}

// Search searches TMDB by title.
func (c *Client) Search(ctx context.Context, q string, kind hermit.Kind) ([]hermit.Hit, error) {
	if c.TMDBKey == "" {
		return nil, fmt.Errorf("TMDB API key not configured; set HERMIT_TMDB_KEY")
	}
	params := url.Values{}
	params.Set("api_key", c.TMDBKey)
	params.Set("query", q)
	if kind == hermit.KindMovie {
		params.Set("include_adult", "false")
	}
	u := c.Base + "/search/multi?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tmdb search HTTP %d", resp.StatusCode)
	}
	var out tmdbSearchResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	var hits []hermit.Hit
	for _, r := range out.Results {
		k := hermit.KindTV
		title := r.Title
		if title == "" {
			title = r.Name
		}
		if r.MediaType == "movie" || r.MediaType == "person" {
			k = hermit.KindMovie
			if r.MediaType == "person" && r.Title == "" && r.Name != "" {
				title = r.Name
			}
		}
		if r.MediaType == "person" {
			k = hermit.KindMovie
		}
		date := r.ReleaseDate
		if date == "" {
			date = r.FirstAirDate
		}
		year := 0
		if len(date) >= 4 {
			fmt.Sscanf(date[:4], "%d", &year)
		}
		hits = append(hits, hermit.Hit{
			Ref:      hermit.Ref{Kind: k, TMDBID: r.ID, Title: title, Year: year, Provider: "tmdb"},
			Title:    title,
			Year:     year,
			Provider: "tmdb",
			Kind:     k,
		})
	}
	return hits, nil
}

// Show fetches details and stores media, seasons, and episodes in SQLite.
func (c *Client) Show(ctx context.Context, ref hermit.Ref) (hermit.Media, []hermit.Season, []hermit.Episode, error) {
	if ref.TMDBID == 0 {
		return hermit.Media{}, nil, nil, fmt.Errorf("TMDB ID required")
	}
	cacheKind := ref.Kind
	if cacheKind == hermit.KindAnime {
		cacheKind = hermit.KindTV
	}
	if cached, err := c.DB.GetMediaByTMDB(ctx, ref.TMDBID, cacheKind); err == nil && !cached.MetaFetchedAt.IsZero() && time.Since(cached.MetaFetchedAt) <= c.CacheTTL {
		var seasons []hermit.Season
		var episodes []hermit.Episode
		if cached.Kind == hermit.KindTV {
			seasons, err = c.DB.GetSeasons(ctx, cached.ID)
			if err != nil {
				return hermit.Media{}, nil, nil, err
			}
			episodes, err = c.DB.GetEpisodes(ctx, cached.ID, 0)
			if err != nil {
				return hermit.Media{}, nil, nil, err
			}
		}
		return cached, seasons, episodes, nil
	}
	media, err := c.showDetails(ctx, ref)
	if err != nil {
		return hermit.Media{}, nil, nil, err
	}
	mediaID, err := c.DB.SaveMedia(ctx, media)
	if err != nil {
		return hermit.Media{}, nil, nil, err
	}
	media.ID = mediaID

	var seasons []hermit.Season
	var episodes []hermit.Episode
	if media.Kind == hermit.KindTV {
		seasons, episodes, err = c.seasonsAndEpisodes(ctx, ref, mediaID)
		if err != nil {
			return media, nil, nil, err
		}
	}
	return media, seasons, episodes, nil
}

func (c *Client) showDetails(ctx context.Context, ref hermit.Ref) (hermit.Media, error) {
	kind := ref.Kind
	if kind == hermit.KindAnime {
		kind = hermit.KindTV
	}
	path := "tv/" + itoa(ref.TMDBID)
	if kind == hermit.KindMovie {
		path = "movie/" + itoa(ref.TMDBID)
	}
	u := c.Base + "/" + path + "?api_key=" + url.QueryEscape(c.TMDBKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return hermit.Media{}, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return hermit.Media{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return hermit.Media{}, fmt.Errorf("tmdb details HTTP %d", resp.StatusCode)
	}
	var raw struct {
		ID             int     `json:"id"`
		Name           string  `json:"name"`
		Title          string  `json:"title"`
		OriginalName   string  `json:"original_name"`
		OriginalTitle  string  `json:"original_title"`
		Overview       string  `json:"overview"`
		PosterPath     string  `json:"poster_path"`
		BackdropPath   string  `json:"backdrop_path"`
		FirstAirDate   string  `json:"first_air_date"`
		ReleaseDate    string  `json:"release_date"`
		Runtime        int     `json:"runtime"`
		NumberSeasons  int     `json:"number_of_seasons"`
		NumberEpisodes int     `json:"number_of_episodes"`
		Genres         []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"genres"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return hermit.Media{}, err
	}
	title := raw.Name
	if title == "" {
		title = raw.Title
	}
	original := raw.OriginalName
	if original == "" {
		original = raw.OriginalTitle
	}
	kindOut := hermit.KindTV
	if ref.Kind == hermit.KindMovie {
		kindOut = hermit.KindMovie
	}
	m := hermit.Media{
		Kind:          kindOut,
		TMDBID:        raw.ID,
		Title:         title,
		OriginalTitle: original,
		Overview:      raw.Overview,
		PosterPath:    raw.PosterPath,
		BackdropPath:  raw.BackdropPath,
		FirstAir:      firstNonEmpty(raw.FirstAirDate, raw.ReleaseDate),
		RuntimeMin:    raw.Runtime,
		SeasonCount:   raw.NumberSeasons,
		EpisodeCount:  raw.NumberEpisodes,
		MetaFetchedAt: time.Now(),
	}
	if len(raw.Genres) > 0 {
		m.Raw = raw.Genres[0].Name
	}
	date := m.FirstAir
	if len(date) >= 4 {
		fmt.Sscanf(date[:4], "%d", &m.Year)
	}
	return m, nil
}

func (c *Client) seasonsAndEpisodes(ctx context.Context, ref hermit.Ref, mediaID int64) ([]hermit.Season, []hermit.Episode, error) {
	var seasons []hermit.Season
	var episodes []hermit.Episode
	u := fmt.Sprintf("%s/tv/%d?api_key=%s", c.Base, ref.TMDBID, url.QueryEscape(c.TMDBKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("tmdb details HTTP %d", resp.StatusCode)
	}
	var raw struct {
		Seasons []struct {
			SeasonNumber int    `json:"season_number"`
			Name         string `json:"name"`
			AirDate      string `json:"air_date"`
			EpisodeCount int    `json:"episode_count"`
		} `json:"seasons"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, nil, err
	}
	for _, s := range raw.Seasons {
		if s.SeasonNumber == 0 {
			continue
		}
		year := 0
		if len(s.AirDate) >= 4 {
			fmt.Sscanf(s.AirDate[:4], "%d", &year)
		}
		season := hermit.Season{MediaID: mediaID, Season: s.SeasonNumber, Name: s.Name, AirYear: year, EpisodeCount: s.EpisodeCount}
		if err := c.DB.SaveSeason(ctx, season); err != nil {
			return nil, nil, err
		}
		seasons = append(seasons, season)
		eps, err := c.seasonEpisodes(ctx, ref.TMDBID, s.SeasonNumber, mediaID)
		if err != nil {
			return nil, nil, err
		}
		episodes = append(episodes, eps...)
	}
	return seasons, episodes, nil
}

func (c *Client) seasonEpisodes(ctx context.Context, tmdb, season int, mediaID int64) ([]hermit.Episode, error) {
	u := fmt.Sprintf("%s/tv/%d/season/%d?api_key=%s", c.Base, tmdb, season, url.QueryEscape(c.TMDBKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tmdb season HTTP %d", resp.StatusCode)
	}
	var raw struct {
		Episodes []struct {
			ID            int    `json:"id"`
			EpisodeNumber int    `json:"episode_number"`
			Name          string `json:"name"`
			AirDate       string `json:"air_date"`
			Runtime       int    `json:"runtime"`
			StillPath     string `json:"still_path"`
		} `json:"episodes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	var out []hermit.Episode
	for _, e := range raw.Episodes {
		ep := hermit.Episode{MediaID: mediaID, Season: season, Episode: e.EpisodeNumber, TMDBEpID: e.ID, Title: e.Name, AirDate: e.AirDate, RuntimeS: e.Runtime, StillPath: e.StillPath}
		if _, err := c.DB.SaveEpisode(ctx, ep); err != nil {
			return nil, err
		}
		out = append(out, ep)
	}
	return out, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// NormalizeKind maps a kind string from CLI to a hermit Kind.
func NormalizeKind(s string) hermit.Kind {
	switch strings.ToLower(s) {
	case "movie", "movies":
		return hermit.KindMovie
	case "anime":
		return hermit.KindAnime
	default:
		return hermit.KindTV
	}
}
