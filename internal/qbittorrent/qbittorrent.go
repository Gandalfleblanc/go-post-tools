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
	// qBit renvoie "Ok." si tout va bien, "Fails." si duplicate / format invalide
	if strings.Contains(bodyStr, "Fails") {
		return "", fmt.Errorf("qBit a refusé le .torrent: %s", bodyStr)
	}

	if onProgress != nil {
		onProgress(Progress{Stage: "done", Percent: 100})
	}
	return fmt.Sprintf("qBit: %s ajouté", filepath.Base(torrentPath)), nil
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
