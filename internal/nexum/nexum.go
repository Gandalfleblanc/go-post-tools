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
		// Tente plusieurs shapes Laravel-pagination ou ad-hoc
		// 1) {"data": [...], "next_page_url": ...}
		// 2) {"pagination": {"data": [...], "next_page_url": ...}}
		// 3) {"torrents": [...]}
		// 4) {"torrents": {"data": [...], "next_page_url": ...}}  ← Laravel paginate() classique
		// 5) [...] tableau brut
		type pag struct {
			Data    []Torrent `json:"data"`
			NextURL *string   `json:"next_page_url"`
			LastPage int       `json:"last_page"`
		}
		var resp struct {
			Data            []Torrent `json:"data"`
			NextURL         *string   `json:"next_page_url"`
			LastPage        int       `json:"last_page"`
			Pagination      *pag      `json:"pagination"`
			TorrentsArr     []Torrent `json:"-"`
			TorrentsPag     *pag      `json:"-"`
			RawTorrents     json.RawMessage `json:"torrents"`
		}
		// Unmarshal d'abord shape 1/2
		_ = json.Unmarshal(data, &resp)
		batch := resp.Data
		var nextURL *string = resp.NextURL
		// Shape 2
		if len(batch) == 0 && resp.Pagination != nil {
			batch = resp.Pagination.Data
			nextURL = resp.Pagination.NextURL
		}
		// Shape 3 ou 4
		if len(batch) == 0 && len(resp.RawTorrents) > 0 {
			// Tente array
			var arr []Torrent
			if e := json.Unmarshal(resp.RawTorrents, &arr); e == nil && len(arr) > 0 {
				batch = arr
			} else {
				// Tente paginate
				var p pag
				if e := json.Unmarshal(resp.RawTorrents, &p); e == nil {
					batch = p.Data
					nextURL = p.NextURL
				}
			}
		}
		// Shape 5 : array brut au top-level
		if len(batch) == 0 {
			var arr []Torrent
			if e := json.Unmarshal(data, &arr); e == nil && len(arr) > 0 {
				batch = arr
			}
		}
		if len(batch) == 0 {
			// Pour debug : remonte un extrait du body si rien trouvé sur la page 1
			if page == 1 {
				return out, fmt.Errorf("réponse Nexum sans torrents (shape inconnue) — extrait : %s", truncate(string(data), 300))
			}
			break
		}
		out = append(out, batch...)
		if nextURL == nil {
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

// ByInfoHash : tente plusieurs endpoints pour retrouver un torrent Nexum
// à partir de son info_hash. Renvoie nil sans erreur si pas trouvé (404).
// Endpoints essayés (le 1er qui répond gagne) :
//   1. /api/v1/torrents/info_hash/<hash>
//   2. /api/v1/torrents?info_hash=<hash>  (filtre côté liste)
//   3. /api/v1/torrents/<hash>            (route by hash en lieu et place de l'id)
func (c *Client) ByInfoHash(hash string) (*Torrent, error) {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if hash == "" {
		return nil, nil
	}
	// Endpoint dédié
	if t, err := c.tryByInfoHashPath(hash); err == nil && t != nil {
		return t, nil
	}
	// Filtre liste
	params := url.Values{}
	params.Set("info_hash", hash)
	if data, err := c.get("/api/v1/torrents", params); err == nil {
		// Tente d'extraire un torrent unique de différentes shapes
		if t := extractFirstTorrent(data); t != nil && t.InfoHash != "" {
			return t, nil
		}
	}
	// Endpoint by-id avec hash
	if t, err := c.tryByInfoHashIDPath(hash); err == nil && t != nil {
		return t, nil
	}
	return nil, nil
}

func (c *Client) tryByInfoHashPath(hash string) (*Torrent, error) {
	data, err := c.get("/api/v1/torrents/info_hash/"+hash, nil)
	if err != nil {
		return nil, err
	}
	return extractSingleTorrent(data), nil
}

func (c *Client) tryByInfoHashIDPath(hash string) (*Torrent, error) {
	data, err := c.get("/api/v1/torrents/"+hash, nil)
	if err != nil {
		return nil, err
	}
	return extractSingleTorrent(data), nil
}

// extractSingleTorrent : essaie {torrent: {...}}, {data: {...}}, {...} direct.
func extractSingleTorrent(data []byte) *Torrent {
	// Direct
	var t Torrent
	if e := json.Unmarshal(data, &t); e == nil && t.InfoHash != "" {
		return &t
	}
	// Wrapped
	var w struct {
		Torrent *Torrent `json:"torrent"`
		Data    *Torrent `json:"data"`
	}
	if e := json.Unmarshal(data, &w); e == nil {
		if w.Torrent != nil && w.Torrent.InfoHash != "" {
			return w.Torrent
		}
		if w.Data != nil && w.Data.InfoHash != "" {
			return w.Data
		}
	}
	return nil
}

// extractFirstTorrent : essaie array ou objet paginé, renvoie le 1er.
func extractFirstTorrent(data []byte) *Torrent {
	// Array brut
	var arr []Torrent
	if e := json.Unmarshal(data, &arr); e == nil && len(arr) > 0 {
		return &arr[0]
	}
	var w struct {
		Data     []Torrent `json:"data"`
		Torrents []Torrent `json:"torrents"`
	}
	if e := json.Unmarshal(data, &w); e == nil {
		if len(w.Data) > 0 {
			return &w.Data[0]
		}
		if len(w.Torrents) > 0 {
			return &w.Torrents[0]
		}
	}
	return nil
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
