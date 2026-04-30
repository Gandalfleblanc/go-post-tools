// Package nextcloud upload des fichiers vers un serveur NextCloud via WebDAV.
//
// Endpoint utilisé : PUT {baseURL}/remote.php/dav/files/{user}/{remotePath}/{filename}
// Auth : HTTP Basic Auth (user + password ou app password).
// TLS : InsecureSkipVerify activé — beaucoup de serveurs NextCloud exposent
// l'API en HTTPS avec un certificat self-signed (IP brute, déploiement perso).
//
// Utilisé par le workflow Torrent ADMIN : upload du MKV vers NextCloud, puis
// qBittorrent ADMIN (qui partage le filesystem avec NextCloud côté serveur)
// hash le fichier et le seed.
package nextcloud

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type Progress struct {
	Percent float64 `json:"percent"`
	SpeedMB float64 `json:"speed_mb"`
}

type progressReader struct {
	r          io.Reader
	total      int64
	read       int64
	start      time.Time
	lastEmit   time.Time
	onProgress func(Progress)
}

func (pr *progressReader) Read(p []byte) (n int, err error) {
	n, err = pr.r.Read(p)
	if n > 0 && pr.onProgress != nil {
		pr.read += int64(n)
		if time.Since(pr.lastEmit) < 250*time.Millisecond {
			return
		}
		pr.lastEmit = time.Now()
		elapsed := time.Since(pr.start).Seconds()
		speed := 0.0
		if elapsed > 0.1 {
			speed = float64(pr.read) / elapsed / 1024 / 1024
		}
		pct := float64(pr.read) / float64(pr.total) * 100
		pr.onProgress(Progress{Percent: math.Min(pct, 99), SpeedMB: speed})
	}
	return
}

// newClient retourne un http.Client qui accepte les certificats self-signed.
// NextCloud déployé sur IP brute (ex: 95.217.107.120) n'a typiquement pas de
// cert valide → on désactive la vérif TLS, justifié pour ce use case interne.
func newClient() *http.Client {
	return &http.Client{
		Timeout: 0, // pas de timeout global — gros MKV peuvent prendre plusieurs heures
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

// webdavURL construit l'URL WebDAV PUT pour un fichier donné.
//
//	baseURL    = https://95.217.107.120 (sans /remote.php)
//	user       = nom d'utilisateur NextCloud
//	remotePath = "/" pour la racine du user, ou "/Hydracker/Torrents/" etc.
//	filename   = nom du fichier (sans path)
//
// Résultat : https://95.217.107.120/remote.php/dav/files/USER/REMOTEPATH/FILENAME
func webdavURL(baseURL, user, remotePath, filename string) string {
	base := strings.TrimRight(baseURL, "/")
	// On enlève /login?... si l'user a copié l'URL du browser
	if i := strings.Index(base, "/login"); i >= 0 {
		base = base[:i]
	}
	if i := strings.Index(base, "/index.php"); i >= 0 {
		base = base[:i]
	}
	// path.Join dégage les // mais ne touche pas le scheme://
	p := path.Join("/remote.php/dav/files", url.PathEscape(user), remotePath, filename)
	return base + p
}

// Upload PUT le fichier mkvPath sur NextCloud via WebDAV.
// Retourne le nom remote du fichier (= basename du fichier source).
func Upload(ctx context.Context, baseURL, user, password, remotePath, mkvPath string, onProgress func(Progress)) (string, error) {
	if baseURL == "" {
		return "", fmt.Errorf("URL NextCloud manquante")
	}
	if user == "" || password == "" {
		return "", fmt.Errorf("credentials NextCloud manquants")
	}
	f, err := os.Open(mkvPath)
	if err != nil {
		return "", fmt.Errorf("open mkv: %w", err)
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat mkv: %w", err)
	}
	filename := filepath.Base(mkvPath)
	u := webdavURL(baseURL, user, remotePath, filename)

	pr := &progressReader{
		r:          f,
		total:      stat.Size(),
		start:      time.Now(),
		lastEmit:   time.Now(),
		onProgress: onProgress,
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", u, pr)
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	req.ContentLength = stat.Size()
	req.SetBasicAuth(user, password)
	req.Header.Set("User-Agent", "GoPostTools/5.x")

	resp, err := newClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("PUT WebDAV: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// 201 Created (nouveau fichier) ou 204 No Content (overwrite) = OK
	if resp.StatusCode != 201 && resp.StatusCode != 204 {
		return "", fmt.Errorf("nextcloud HTTP %d: %s", resp.StatusCode, truncate(string(body), 300))
	}
	if onProgress != nil {
		onProgress(Progress{Percent: 100, SpeedMB: 0})
	}
	return filename, nil
}

// Ping teste la connexion + auth (PROPFIND sur la racine WebDAV du user).
func Ping(baseURL, user, password string) error {
	if baseURL == "" {
		return fmt.Errorf("URL NextCloud manquante")
	}
	if user == "" || password == "" {
		return fmt.Errorf("credentials NextCloud manquants")
	}
	u := webdavURL(baseURL, user, "/", "")
	// On veut juste tester l'accès, on supprime le trailing slash en trop
	u = strings.TrimRight(u, "/")
	req, err := http.NewRequest("PROPFIND", u, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(user, password)
	req.Header.Set("Depth", "0")
	req.Header.Set("User-Agent", "GoPostTools/5.x")

	c := newClient()
	c.Timeout = 15 * time.Second
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("PROPFIND: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return fmt.Errorf("nextcloud: credentials invalides (401)")
	}
	// 207 Multi-Status = succès WebDAV. 200 = aussi OK pour certains setups.
	if resp.StatusCode != 207 && resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("nextcloud HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
