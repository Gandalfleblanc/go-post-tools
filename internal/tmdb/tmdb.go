// Package tmdb : client pour proxytmdb (https://tmdb.uklm.xyz) — un proxy
// qui cache TMDB + fusionne les notes IMDb + JustWatch providers, sans
// nécessiter de clé API côté client.
//
// Endpoints utilisés :
//   - GET /api?t=search&q=<title> [year/SxxExx requis dans la query]
//   - GET /api?t=movie&q=<tmdb_id>  → JSON TMDB standard
//   - GET /api?t=tv&q=<tmdb_id>     → JSON TMDB standard (sériés)
//   - GET /api?t=imdb&q=<imdb_id>   → lookup IMDb → fiche TMDB
//   - GET /api?t=providers&type=movie|tv&q=<tmdb_id>
package tmdb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBase = "https://tmdb.uklm.xyz"

type Client struct {
	base       string
	httpClient *http.Client
}

// NewClient garde la signature compatible avec l'ancien code (apiKey ignoré).
func NewClient(_ string) *Client {
	return &Client{base: defaultBase + "/api", httpClient: &http.Client{Timeout: 15 * time.Second}}
}

// NewClientWithBase permet de pointer vers un autre proxy TMDB (custom par user).
func NewClientWithBase(baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBase
	}
	baseURL = strings.TrimRight(baseURL, "/")
	// Si l'user a donné l'URL racine, on ajoute /api
	if !strings.HasSuffix(baseURL, "/api") {
		baseURL += "/api"
	}
	return &Client{base: baseURL, httpClient: &http.Client{Timeout: 15 * time.Second}}
}

// Movie : structure compatible TMDB officiel + champs bonus du proxy
// (ImdbID, NoteImdb, VoteImdb).
type Movie struct {
	ID           int     `json:"id"`
	Title        string  `json:"title"`
	Name         string  `json:"name"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	ReleaseDate  string  `json:"release_date"`
	FirstAirDate string  `json:"first_air_date"`
	VoteAverage  float64 `json:"vote_average"`
	MediaType    string  `json:"media_type"`
	ImdbID       string  `json:"imdb_id,omitempty"`
	NoteImdb     float64 `json:"note_imdb,omitempty"`
	VoteImdb     int     `json:"vote_imdb,omitempty"`
}

func (m *Movie) DisplayTitle() string {
	if m.Title != "" {
		return m.Title
	}
	return m.Name
}

func (m *Movie) Year() string {
	d := m.ReleaseDate
	if d == "" {
		d = m.FirstAirDate
	}
	if len(d) >= 4 {
		return d[:4]
	}
	return ""
}

func (m *Movie) PosterURL() string {
	if m.PosterPath == "" {
		return ""
	}
	return "https://image.tmdb.org/t/p/w500" + m.PosterPath
}

// searchHit : shape interne renvoyée par /api?t=search (différente de TMDB).
type searchHit struct {
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	Years         int     `json:"years"`
	PosterPath    string  `json:"poster_path"`
	Genres        string  `json:"genres"`
	Runtime       string  `json:"runtime"`
	ImdbID        string  `json:"imdb_id"`
	NoteImdb      float64 `json:"note_imdb"`
	VoteImdb      int     `json:"vote_imdb"`
	TmdbID        int     `json:"tmdb_id"`
	NoteTmdb      float64 `json:"note_tmdb"`
	Overview      string  `json:"overview"`
	MediaType     string  `json:"media_type,omitempty"` // "movie" ou "tv" si proxy l'expose
}

func (h searchHit) toMovie() Movie {
	m := Movie{
		ID:          h.TmdbID,
		Title:       h.Title,
		Overview:    h.Overview,
		PosterPath:  h.PosterPath,
		VoteAverage: h.NoteTmdb,
		MediaType:   h.MediaType,
		ImdbID:      h.ImdbID,
		NoteImdb:    h.NoteImdb,
		VoteImdb:    h.VoteImdb,
	}
	if h.Years > 0 {
		m.ReleaseDate = fmt.Sprintf("%d-01-01", h.Years)
	}
	if m.MediaType == "" {
		m.MediaType = "movie" // défaut
	}
	return m
}

// Search : hit /api?t=search. La query DOIT contenir une année (ex: "Tarzan 1999")
// OU un pattern d'épisode (S01E01, NxN, etc.) sinon le proxy renvoie une erreur.
func (c *Client) Search(query string) ([]Movie, error) {
	params := url.Values{}
	params.Set("t", "search")
	params.Set("q", query)
	req, err := http.NewRequest("GET", c.base+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GoPostTools/4.x")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("proxytmdb HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var resBody struct {
		Results []searchHit `json:"results"`
		Error   string      `json:"error"`
	}
	if err := json.Unmarshal(body, &resBody); err != nil {
		return nil, fmt.Errorf("proxytmdb parse: %w", err)
	}
	if resBody.Error != "" {
		return nil, fmt.Errorf("proxytmdb: %s", resBody.Error)
	}
	out := make([]Movie, 0, len(resBody.Results))
	for _, h := range resBody.Results {
		out = append(out, h.toMovie())
	}
	return out, nil
}

// GetByID : récupère le détail TMDB d'un movie/tv. Le proxy renvoie le JSON
// TMDB standard (compatible avec notre struct Movie sans adapter).
func (c *Client) GetByID(id int, mediaType string) (*Movie, error) {
	if mediaType == "" {
		mediaType = "movie"
	}
	if mediaType != "movie" && mediaType != "tv" {
		mediaType = "movie"
	}
	params := url.Values{}
	params.Set("t", mediaType)
	params.Set("q", fmt.Sprintf("%d", id))
	req, err := http.NewRequest("GET", c.base+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GoPostTools/4.x")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("proxytmdb HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var movie Movie
	movie.MediaType = mediaType
	if err := json.Unmarshal(body, &movie); err != nil {
		return nil, err
	}
	return &movie, nil
}

// GetByImdbID : lookup direct par IMDb ID (ex: "tt0120855") → fiche TMDB.
// Bonus offert par proxytmdb pour matcher rapidement Hydracker ↔ TMDB
// quand la fiche Hydracker a un imdb_id.
func (c *Client) GetByImdbID(imdbID string) (*Movie, error) {
	params := url.Values{}
	params.Set("t", "imdb")
	params.Set("q", imdbID)
	req, err := http.NewRequest("GET", c.base+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GoPostTools/4.x")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("proxytmdb imdb HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var movie Movie
	if err := json.Unmarshal(body, &movie); err != nil {
		return nil, err
	}
	if movie.FirstAirDate != "" {
		movie.MediaType = "tv"
	} else {
		movie.MediaType = "movie"
	}
	return &movie, nil
}

// Provider : un service de streaming (Netflix, Disney+, etc.).
type Provider struct {
	LogoPath     string `json:"logo_path"`
	ProviderID   int    `json:"provider_id"`
	ProviderName string `json:"provider_name"`
}

// CountryProviders : ce qui est dispo dans un pays donné.
type CountryProviders struct {
	Link      string     `json:"link"`
	Flatrate  []Provider `json:"flatrate,omitempty"` // streaming inclus dans abonnement
	Buy       []Provider `json:"buy,omitempty"`
	Rent      []Provider `json:"rent,omitempty"`
	Free      []Provider `json:"free,omitempty"`
}

// GetProviders : où regarder un movie/tv en streaming, par pays.
// Retourne map[country_code]CountryProviders. Pour la France, key = "FR".
func (c *Client) GetProviders(tmdbID int, mediaType string) (map[string]CountryProviders, error) {
	if mediaType == "" {
		mediaType = "movie"
	}
	params := url.Values{}
	params.Set("t", "providers")
	params.Set("type", mediaType)
	params.Set("q", fmt.Sprintf("%d", tmdbID))
	req, err := http.NewRequest("GET", c.base+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GoPostTools/4.x")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("proxytmdb providers HTTP %d", resp.StatusCode)
	}
	var resBody struct {
		Results map[string]CountryProviders `json:"results"`
	}
	if err := json.Unmarshal(body, &resBody); err != nil {
		return nil, err
	}
	return resBody.Results, nil
}

// TestConnection : ping le serveur via /health (endpoint léger).
func (c *Client) TestConnection() error {
	req, err := http.NewRequest("GET", "https://tmdb.uklm.xyz/health", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "GoPostTools/4.x")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("proxytmdb HTTP %d", resp.StatusCode)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
