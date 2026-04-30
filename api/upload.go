package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

type UploadTorrentResult struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	DownloadURL string `json:"download_url"`
	ExpiresAt   string `json:"expires_at"`
	Torrent     struct {
		ID     int    `json:"id"`
		Hash   string `json:"hash"`
		Active bool   `json:"active"`
		// TitleID/Qualite retirés : l'API peut les renvoyer en int OU string
		// selon les versions — l'ancien tag ,string faisait planter le parsing
		// quand l'API renvoyait un int, causant un retry (double post).
		// On les connaît déjà côté caller, pas besoin de les lire ici.
	} `json:"torrent"`
}

type UploadNzbResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Nzb     struct {
		ID     int         `json:"id"`
		Active interface{} `json:"active"`
	} `json:"nzb"`
}

type UploadLienItem struct {
	ID     int         `json:"id"`
	Active interface{} `json:"active"`
	URL    string      `json:"lien"`
}

type UploadLienResult struct {
	Status string           `json:"status"`
	Liens  []UploadLienItem `json:"liens"`
}

// Lien retourne le premier lien posté (ou une struct vide si aucun).
func (r *UploadLienResult) Lien() UploadLienItem {
	if len(r.Liens) > 0 {
		return r.Liens[0]
	}
	return UploadLienItem{}
}

func (c *Client) UploadTorrent(titleID, qualite int, langues, subs []string, torrentPath, nfo string, saison, episode int, fullSaison bool) (*UploadTorrentResult, error) {
	f, err := os.Open(torrentPath)
	if err != nil {
		return nil, fmt.Errorf("impossible d'ouvrir le fichier torrent: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	part, err := w.CreateFormFile("torrent", filepath.Base(torrentPath))
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(part, f); err != nil {
		return nil, err
	}

	_ = w.WriteField("title_id", strconv.Itoa(titleID))
	_ = w.WriteField("qualite", strconv.Itoa(qualite))
	for _, l := range langues {
		_ = w.WriteField("langues[]", l)
	}
	for _, s := range subs {
		_ = w.WriteField("subs[]", s)
	}
	if saison > 0 {
		_ = w.WriteField("saison", strconv.Itoa(saison))
	}
	if episode > 0 && !fullSaison {
		_ = w.WriteField("episode", strconv.Itoa(episode))
	}
	if fullSaison {
		_ = w.WriteField("full_saison", "1")
	}
	if nfo != "" {
		_ = w.WriteField("nfo", nfo)
	}
	w.Close()

	data, err := c.doMultipart("/torrents", &buf, w.FormDataContentType())
	if err != nil {
		return nil, err
	}
	var result UploadTorrentResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UploadNzb(titleID, qualite int, langues, subs []string, nzbPath, nfo string, saison, episode int, fullSaison bool) (*UploadNzbResult, error) {
	f, err := os.Open(nzbPath)
	if err != nil {
		return nil, fmt.Errorf("impossible d'ouvrir le fichier NZB: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	part, err := w.CreateFormFile("nzb", filepath.Base(nzbPath))
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(part, f); err != nil {
		return nil, err
	}

	_ = w.WriteField("title_id", strconv.Itoa(titleID))
	_ = w.WriteField("qualite", strconv.Itoa(qualite))
	for _, l := range langues {
		_ = w.WriteField("langues[]", l)
	}
	for _, s := range subs {
		_ = w.WriteField("subs[]", s)
	}
	if saison > 0 {
		_ = w.WriteField("saison", strconv.Itoa(saison))
	}
	if episode > 0 && !fullSaison {
		_ = w.WriteField("episode", strconv.Itoa(episode))
	}
	if fullSaison {
		_ = w.WriteField("full_saison", "1")
	}
	if nfo != "" {
		_ = w.WriteField("nfo", nfo)
	}
	w.Close()

	data, err := c.doMultipart("/nzb", &buf, w.FormDataContentType())
	if err != nil {
		return nil, err
	}
	var result UploadNzbResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UploadLien(titleID, qualite int, langues, subs []string, lien, nfo string, saison, episode int, fullSaison bool) (*UploadLienResult, error) {
	// Format réel du site Hydracker (use-create-lien.ts) :
	// - typefile:"DDL" obligatoire
	// - start_epi = numéro d'épisode (string), episode = nombre d'épisodes couverts (int, 1 pour un seul)
	// - full_saison = 1 si saison complète, sinon 0
	payload := map[string]any{
		"title_id":    titleID,
		"typefile":    "DDL",
		"qualite":     qualite,
		"langues":     langues,
		"lien":        lien,
		"full_saison": 0,
		"episode":     1,
	}
	if fullSaison {
		payload["full_saison"] = 1
	}
	if len(subs) > 0 {
		payload["subs"] = subs
	}
	if saison > 0 {
		payload["saison"] = saison
	}
	if episode > 0 && !fullSaison {
		payload["start_epi"] = strconv.Itoa(episode)
	}
	if nfo != "" {
		payload["nfo"] = nfo
	}
	data, err := c.post("/liens", payload)
	if err != nil {
		return nil, err
	}
	var result UploadLienResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) doMultipart(path string, body *bytes.Buffer, contentType string) ([]byte, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("URL Hydracker non configurée (Réglages)")
	}
	req, err := http.NewRequestWithContext(c.getContext(), "POST", c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		return data, nil
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("unauthorized: vérifiez votre token")
	case http.StatusForbidden:
		return nil, fmt.Errorf("accès refusé (403) — permission manquante")
	case http.StatusUnprocessableEntity:
		return nil, fmt.Errorf("données invalides (422): %s", string(data))
	default:
		return nil, fmt.Errorf("erreur HTTP %d: %s", resp.StatusCode, string(data))
	}
}
