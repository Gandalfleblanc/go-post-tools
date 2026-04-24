// Package nexum : client API pour Nexum (tracker secondaire).
// Auth : header X-API-Key. Doc : https://nexum-core.com/api-docs
package nexum

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBase = "https://nexum-core.com"

type Torrent struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	InfoHash  string `json:"info_hash"`
	Size      int64  `json:"size"`
	Seeders   int    `json:"seeders"`
	Leechers  int    `json:"leechers"`
	Completed int    `json:"completed"`
	CreatedAt string `json:"created_at"`
}

type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func NewClient(apiKey, baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBase
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) get(path string, params url.Values) ([]byte, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("clé API Nexum manquante")
	}
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("User-Agent", "GoPostTools/4.x")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("nexum HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return body, nil
}

// Me : ping pour vérifier que la clé est valide.
type AccountInfo struct {
	ID       int     `json:"id"`
	Username string  `json:"username"`
	Role     string  `json:"role"`
	Ratio    float64 `json:"ratio"`
}

func (c *Client) Me() (*AccountInfo, error) {
	data, err := c.get("/api/v1/me", nil)
	if err != nil {
		return nil, err
	}
	var info AccountInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// ListAll : retourne TOUS les torrents (paginate jusqu'au bout).
// Pour 1000+ torrents ça prend du temps — privilégier ListAllByMe pour
// filtrer côté serveur si possible.
func (c *Client) ListAll() ([]Torrent, error) {
	out := []Torrent{}
	page := 1
	for {
		params := url.Values{}
		params.Set("page", fmt.Sprintf("%d", page))
		params.Set("per_page", "100")
		params.Set("sort", "created_at")
		params.Set("dir", "desc")
		data, err := c.get("/api/v1/torrents", params)
		if err != nil {
			return out, err
		}
		var resp struct {
			Data         []Torrent `json:"data"`
			Pagination   *struct {
				Data    []Torrent `json:"data"`
				NextURL *string   `json:"next_page_url"`
				LastPage int       `json:"last_page"`
			} `json:"pagination"`
			Torrents []Torrent `json:"torrents"`
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			return out, err
		}
		// Le serveur peut renvoyer plusieurs shapes — on essaie chacune
		batch := resp.Data
		if len(batch) == 0 && resp.Pagination != nil {
			batch = resp.Pagination.Data
		}
		if len(batch) == 0 {
			batch = resp.Torrents
		}
		if len(batch) == 0 {
			break
		}
		out = append(out, batch...)
		// Stop si on est à la dernière page (heuristique)
		if resp.Pagination != nil && resp.Pagination.NextURL == nil {
			break
		}
		if len(batch) < 100 {
			break
		}
		page++
		if page > 100 { // safety : 10k torrents max
			break
		}
	}
	return out, nil
}

// ReleasesByTmdbID : alternative + précise — retourne les releases Nexum
// pour un TMDB id donné. Utile pour matcher rapidement Hydracker → Nexum
// quand on a le tmdb_id côté Hydracker.
func (c *Client) ReleasesByTmdbID(tmdbID int) ([]Torrent, error) {
	data, err := c.get(fmt.Sprintf("/api/v1/tmdb/%d/releases", tmdbID), nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Releases []Torrent `json:"releases"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return resp.Releases, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
