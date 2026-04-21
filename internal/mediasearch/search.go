// Package mediasearch parse les résultats d'un endpoint de recherche film/série
// (renvoie tmdb_id + métadonnées) — endpoint configurable par l'utilisateur.
package mediasearch

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type SearchResult struct {
	TmdbID    int    `json:"tmdb_id"`
	MediaType string `json:"media_type"` // "movie" | "tv"
	TitleFR   string `json:"title_fr"`
	TitleVO   string `json:"title_vo"`
	Year      string `json:"year"`
	PosterURL string `json:"poster_url"`
}

var (
	// Chaque résultat est encapsulé dans un <span style="display: inline-block; margin: 10px; ...">
	reResultBlock = regexp.MustCompile(`(?s)<span style="display: inline-block; margin: 10px;.*?</span></span>`)
	reTmdbURL     = regexp.MustCompile(`themoviedb\.org/(movie|tv)/(\d+)`)
	reTitleFR     = regexp.MustCompile(`(?s)FR\s*<b>([^<]+)</b>\s*(\d{4})`)
	reTitleVO     = regexp.MustCompile(`(?s)VO\s*<b>([^<]+)</b>\s*(\d{4})`)
	rePoster      = regexp.MustCompile(`<img\s+src="([^"]+)"`)
)

// Search interroge le endpoint de recherche configuré pour trouver un film/série.
// searchURL doit se terminer par ?query= ou similaire — query sera concaténée après.
func Search(searchURL, query string) ([]SearchResult, error) {
	// Si URL non configurée, renvoie liste vide sans erreur pour que le
	// caller puisse fallback sur TMDB direct sans log d'erreur parasite.
	if searchURL == "" {
		return nil, nil
	}
	q := url.QueryEscape(query)
	u := searchURL + q

	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15")

	c := &http.Client{Timeout: 15 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	html := string(body)

	if strings.Contains(html, "Year or episode not found") {
		return nil, fmt.Errorf("ajoutez l'année à la requête (ex: \"Titre 2023\")")
	}

	// On extrait chaque bloc de résultat
	blocks := reResultBlock.FindAllString(html, -1)
	if len(blocks) == 0 {
		// Fallback : si pas de bloc délimité, on essaie sur tout le HTML
		blocks = []string{html}
	}

	var results []SearchResult
	for _, b := range blocks {
		m := reTmdbURL.FindStringSubmatch(b)
		if m == nil {
			continue
		}
		tmdbID, _ := strconv.Atoi(m[2])
		r := SearchResult{TmdbID: tmdbID, MediaType: m[1]}
		if fr := reTitleFR.FindStringSubmatch(b); fr != nil {
			r.TitleFR = strings.TrimSpace(fr[1])
			r.Year = fr[2]
		}
		if vo := reTitleVO.FindStringSubmatch(b); vo != nil {
			r.TitleVO = strings.TrimSpace(vo[1])
			if r.Year == "" {
				r.Year = vo[2]
			}
		}
		if p := rePoster.FindStringSubmatch(b); p != nil {
			r.PosterURL = p[1]
		}
		results = append(results, r)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("aucun résultat pour « %s »", query)
	}
	return results, nil
}
