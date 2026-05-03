package main

// Fallback de vérification TMDB via serveurperso.com (scrape HTML).
// Sert uniquement quand UKLM est down — pour confirmer que l'ID existe
// avant d'orienter l'utilisateur, on ne parse pas la fiche complète.

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"
)

// serveurpersoCheckTMDB : interroge serveurperso et confirme que l'ID
// est référencé dans la réponse HTML. Retourne (exists, error).
func serveurpersoCheckTMDB(tmdbID int) (bool, error) {
	url := fmt.Sprintf("https://www.serveurperso.com/stats/search.php?query=%d", tmdbID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", "GoPostTools-AutoPost/1.0")
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false, fmt.Errorf("serveurperso HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return false, err
	}
	needle := []byte(fmt.Sprintf("/movie/%d", tmdbID))
	return bytes.Contains(body, needle), nil
}
