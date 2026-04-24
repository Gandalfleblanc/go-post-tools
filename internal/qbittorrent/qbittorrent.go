// Package qbittorrent fournit les opérations seedbox via l'API Web UI v2 de
// qBittorrent. Équivalent fonctionnel du package rutorrent/seedbox pour les
// seedbox qui utilisent qBit (notamment la seedbox modérateur Hydracker).
//
// Auth : POST /api/v2/auth/login avec form {username, password}. Le serveur
// retourne "Ok." et un cookie SID qu'on réutilise pour les requêtes suivantes.
//
// Upload : POST /api/v2/torrents/add (multipart) avec le fichier + options.
// Recheck : POST /api/v2/torrents/recheck (form {hashes}).
package qbittorrent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anacrolix/torrent/metainfo"
)

// Progress rapporte l'avancement de l'upload vers qBit.
type Progress struct {
	Stage   string  `json:"stage"`
	Percent float64 `json:"percent"`
	SpeedMB float64 `json:"speed_mb"`
}

// client privé — créé dans chaque call pour porter le cookie SID.
func newClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Timeout: 60 * time.Second,
		Jar:     jar,
	}
}

// login effectue POST /api/v2/auth/login, retourne "Ok." en body si succès.
// Le cookie SID est conservé automatiquement par le cookie jar du client.
func login(ctx context.Context, c *http.Client, baseURL, user, password string) error {
	u := strings.TrimRight(baseURL, "/") + "/api/v2/auth/login"
	form := url.Values{}
	form.Set("username", user)
	form.Set("password", password)
	req, _ := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", strings.TrimRight(baseURL, "/"))
	req.Header.Set("User-Agent", "GoPostTools/3.3")
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("login qbit: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := strings.TrimSpace(string(body))
	if resp.StatusCode != 200 {
		return fmt.Errorf("login qbit HTTP %d: %s", resp.StatusCode, bodyStr)
	}
	if !strings.HasPrefix(bodyStr, "Ok") {
		return fmt.Errorf("login qbit refusé: %s", bodyStr)
	}
	return nil
}

// Ping : test de login, utilisé par tester.TestQBit.
func Ping(baseURL, user, password string) error {
	c := newClient()
	return login(context.Background(), c, baseURL, user, password)
}

// Upload ajoute un .torrent sur qBit via /api/v2/torrents/add.
// Le retour onProgress n'est pas super utile ici (l'upload du .torrent est
// minuscule, quelques KB), mais on émet quand même des events stage pour le UI.
func Upload(ctx context.Context, baseURL, user, password, label, torrentPath string, onProgress func(Progress)) (string, error) {
	if baseURL == "" {
		return "", fmt.Errorf("URL qBit manquante")
	}
	c := newClient()

	if onProgress != nil {
		onProgress(Progress{Stage: "login"})
	}
	if err := login(ctx, c, baseURL, user, password); err != nil {
		return "", err
	}

	// Lit le fichier .torrent
	f, err := os.Open(torrentPath)
	if err != nil {
		return "", fmt.Errorf("open torrent: %w", err)
	}
	defer f.Close()

	// Construit le body multipart
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Champ torrents= (le fichier)
	fw, err := w.CreateFormFile("torrents", filepath.Base(torrentPath))
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return "", fmt.Errorf("copy torrent: %w", err)
	}

	// Options : catégorie = label, paused=false, autoTMM=false (pour que qBit
	// utilise le save path par défaut du serveur où les MKV FTP sont uploadés).
	if label != "" {
		_ = w.WriteField("category", label)
	}
	_ = w.WriteField("autoTMM", "false")
	_ = w.WriteField("paused", "false")
	_ = w.WriteField("rename", "") // pas de renommage
	_ = w.Close()

	if onProgress != nil {
		onProgress(Progress{Stage: "upload", Percent: 50})
	}

	u := strings.TrimRight(baseURL, "/") + "/api/v2/torrents/add"
	req, _ := http.NewRequestWithContext(ctx, "POST", u, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Referer", strings.TrimRight(baseURL, "/"))
	req.Header.Set("User-Agent", "GoPostTools/3.3")
	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("add torrent qbit: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("add torrent qbit HTTP %d: %s", resp.StatusCode, string(body))
	}
	bodyStr := strings.TrimSpace(string(body))
	// qBit renvoie "Ok." si tout va bien, "Fails." si duplicate / format invalide.
	// Comme qBit ne distingue pas les 2 cas dans la réponse, on vérifie après coup
	// si le torrent est bien présent (via son info_hash) — si oui, c'est un
	// duplicate benign, on considère ça comme succès.
	if strings.Contains(bodyStr, "Fails") {
		if mi, merr := metainfo.LoadFromFile(torrentPath); merr == nil {
			hash := strings.ToLower(mi.HashInfoBytes().HexString())
			if isPresent(ctx, c, baseURL, hash) {
				if onProgress != nil {
					onProgress(Progress{Stage: "done", Percent: 100})
				}
				return fmt.Sprintf("qBit: %s déjà présent (duplicate OK)", filepath.Base(torrentPath)), nil
			}
		}
		return "", fmt.Errorf("qBit a refusé le .torrent: %s", bodyStr)
	}

	if onProgress != nil {
		onProgress(Progress{Stage: "done", Percent: 100})
	}
	return fmt.Sprintf("qBit: %s ajouté", filepath.Base(torrentPath)), nil
}

// isPresent vérifie si un torrent est déjà présent sur qBit via
// GET /api/v2/torrents/info?hashes=X. Retourne true si présent.
func isPresent(ctx context.Context, c *http.Client, baseURL, hash string) bool {
	u := strings.TrimRight(baseURL, "/") + "/api/v2/torrents/info?hashes=" + url.QueryEscape(hash)
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("Referer", strings.TrimRight(baseURL, "/"))
	req.Header.Set("User-Agent", "GoPostTools/3.3")
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// qBit renvoie un JSON array. Si le hash est trouvé → non-vide comme "[{...}]".
	// Si pas trouvé → "[]".
	trimmed := strings.TrimSpace(string(body))
	return resp.StatusCode == 200 && trimmed != "" && trimmed != "[]"
}

// ListHashes retourne tous les info_hashes (lowercase) présents sur qBit.
// Utilisé par Check Torrent pour ne montrer que les torrents que l'user
// a réellement encore en seed (filtre sa liste Hydracker).
func ListHashes(baseURL, user, password string) ([]string, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("URL qBit manquante")
	}
	c := newClient()
	ctx := context.Background()
	if err := login(ctx, c, baseURL, user, password); err != nil {
		return nil, err
	}
	u := strings.TrimRight(baseURL, "/") + "/api/v2/torrents/info"
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("Referer", strings.TrimRight(baseURL, "/"))
	req.Header.Set("User-Agent", "GoPostTools/4.x")
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list qbit: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("list qbit HTTP %d", resp.StatusCode)
	}
	var items []struct {
		Hash string `json:"hash"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("parse qbit list: %w", err)
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it.Hash != "" {
			out = append(out, strings.ToLower(it.Hash))
		}
	}
	return out, nil
}

// Delete supprime un torrent de qBit. Si deleteFiles=true, supprime aussi
// les fichiers sur disque. Idempotent (succès même si le hash n'existe pas).
func Delete(baseURL, user, password, hash string, deleteFiles bool) error {
	if baseURL == "" {
		return fmt.Errorf("URL qBit manquante")
	}
	c := newClient()
	ctx := context.Background()
	if err := login(ctx, c, baseURL, user, password); err != nil {
		return err
	}
	u := strings.TrimRight(baseURL, "/") + "/api/v2/torrents/delete"
	form := url.Values{}
	form.Set("hashes", strings.ToLower(hash))
	if deleteFiles {
		form.Set("deleteFiles", "true")
	} else {
		form.Set("deleteFiles", "false")
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", strings.TrimRight(baseURL, "/"))
	req.Header.Set("User-Agent", "GoPostTools/4.x")
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("delete qbit: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete qbit HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// Recheck force un recheck sur le torrent identifié par son info_hash.
// POST /api/v2/torrents/recheck avec form hashes=xxx
func Recheck(baseURL, user, password, hash string) error {
	if baseURL == "" {
		return fmt.Errorf("URL qBit manquante")
	}
	c := newClient()
	ctx := context.Background()
	if err := login(ctx, c, baseURL, user, password); err != nil {
		return err
	}
	u := strings.TrimRight(baseURL, "/") + "/api/v2/torrents/recheck"
	form := url.Values{}
	form.Set("hashes", hash)
	req, _ := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", strings.TrimRight(baseURL, "/"))
	req.Header.Set("User-Agent", "GoPostTools/3.3")
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("recheck qbit: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("recheck qbit HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
