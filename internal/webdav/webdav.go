// Package webdav upload des fichiers via PUT WebDAV (compatible Nextcloud,
// ownCloud, filebrowser, etc.). Utilisé pour la Seedbox Modérateur où le
// fichier doit être déposé via l'interface web avant d'être synchronisé
// avec le seedbox BitTorrent côté serveur.
//
// Nextcloud/ownCloud chemin WebDAV standard :
//
//	PUT {baseURL}/remote.php/dav/files/{username}/{path}/{filename}
//
// Certains providers exposent aussi :
//
//	PUT {baseURL}/webdav/{path}/{filename}
package webdav

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Progress struct {
	Bytes   int64   `json:"bytes"`
	Total   int64   `json:"total"`
	Percent float64 `json:"percent"`
	SpeedMB float64 `json:"speed_mb"`
}

// progressReader wrappe un io.Reader et émet onProgress toutes les 250ms.
type progressReader struct {
	r          io.Reader
	total      int64
	onProgress func(Progress)
	bytes      int64
	start      time.Time
	lastEmit   time.Time
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	if n > 0 {
		pr.bytes += int64(n)
		now := time.Now()
		if pr.start.IsZero() {
			pr.start = now
			pr.lastEmit = now
		}
		if now.Sub(pr.lastEmit) >= 250*time.Millisecond && pr.onProgress != nil {
			pr.lastEmit = now
			elapsed := now.Sub(pr.start).Seconds()
			var speed float64
			if elapsed > 0 {
				speed = float64(pr.bytes) / elapsed / 1e6
			}
			var pct float64
			if pr.total > 0 {
				pct = float64(pr.bytes) / float64(pr.total) * 100
				if pct > 99 {
					pct = 99
				}
			}
			pr.onProgress(Progress{Bytes: pr.bytes, Total: pr.total, Percent: pct, SpeedMB: speed})
		}
	}
	return n, err
}

// chunkSize : taille d'un chunk pour le protocole Nextcloud chunked upload.
// 10MB est un bon compromis : assez petit pour passer les limites nginx
// (typiquement 40MB-100MB) et assez grand pour pas multiplier les requêtes.
const chunkSize = 10 * 1024 * 1024

// Upload envoie un fichier local vers le serveur Nextcloud via le protocole
// legacy "OC-Chunked" v1. Compatible avec les providers qui :
//  - ont une limite nginx sur body size (413 sur single PUT gros fichiers)
//  - bloquent la méthode PUT sur /remote.php/dav/uploads/... (405) donc le
//    chunked v2 (MKCOL + PUT + MOVE) ne passe pas
//
// Protocole v1 :
//   PUT {base}/remote.php/dav/files/{user}/{path}/{filename}-chunking-{transferId}-{totalChunks}-{chunkIdx}
//   avec Header OC-Chunked: 1
//   Le serveur assemble automatiquement le fichier final quand le dernier
//   chunk est reçu.
func Upload(ctx context.Context, baseURL, username, password, remotePath, filename, localPath string, onProgress func(Progress)) (string, error) {
	if baseURL == "" {
		return "", fmt.Errorf("URL WebDAV manquante")
	}
	base := strings.TrimRight(baseURL, "/")
	userEsc := url.PathEscape(username)

	f, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", localPath, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", localPath, err)
	}
	total := info.Size()

	p := strings.Trim(remotePath, "/")
	if p != "" {
		p = "/" + p
	}
	// Path legacy /remote.php/webdav/ (sans /dav/files/{user}/) — accepté par
	// certains providers qui bloquent le dav v2. Même API Nextcloud derrière.
	_ = userEsc
	finalURL := base + "/remote.php/webdav" + p + "/" + url.PathEscape(filename)

	totalChunks := int((total + chunkSize - 1) / chunkSize)
	if totalChunks < 1 {
		totalChunks = 1
	}
	transferID := fmt.Sprintf("%d", time.Now().UnixNano())

	c := &http.Client{Timeout: 0}
	var uploaded int64
	start := time.Now()
	lastEmit := start

	buf := make([]byte, chunkSize)
	for i := 0; i < totalChunks; i++ {
		n, rerr := io.ReadFull(f, buf)
		if n == 0 && rerr != nil {
			return "", fmt.Errorf("read chunk %d: %w", i, rerr)
		}
		chunkURL := fmt.Sprintf("%s-chunking-%s-%d-%d", finalURL, transferID, totalChunks, i)
		req, err := http.NewRequestWithContext(ctx, "PUT", chunkURL, strings.NewReader(string(buf[:n])))
		if err != nil {
			return "", err
		}
		req.ContentLength = int64(n)
		req.SetBasicAuth(username, password)
		req.Header.Set("User-Agent", "GoPostTools/3.3")
		req.Header.Set("OC-Chunked", "1")
		// OC-Total-Length est utile pour aider le serveur à valider l'assemblage final.
		req.Header.Set("OC-Total-Length", fmt.Sprintf("%d", total))
		resp, err := c.Do(req)
		if err != nil {
			return "", fmt.Errorf("PUT chunk %d: %w", i+1, err)
		}
		rb, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		// Codes de succès : 201 Created, 204 No Content.
		if resp.StatusCode != 201 && resp.StatusCode != 204 && resp.StatusCode != 200 {
			preview := string(rb)
			if len(preview) > 300 {
				preview = preview[:300] + "…"
			}
			return "", fmt.Errorf("PUT chunk %d/%d HTTP %d: %s", i+1, totalChunks, resp.StatusCode, preview)
		}
		uploaded += int64(n)
		now := time.Now()
		if onProgress != nil && (now.Sub(lastEmit) >= 250*time.Millisecond || uploaded == total) {
			lastEmit = now
			elapsed := now.Sub(start).Seconds()
			var speed, pct float64
			if elapsed > 0 {
				speed = float64(uploaded) / elapsed / 1e6
			}
			if total > 0 {
				pct = float64(uploaded) / float64(total) * 100
				if pct > 99 && uploaded < total {
					pct = 99
				}
			}
			onProgress(Progress{Bytes: uploaded, Total: total, Percent: pct, SpeedMB: speed})
		}
		if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
			break
		}
	}

	if onProgress != nil {
		onProgress(Progress{Bytes: total, Total: total, Percent: 100})
	}
	return finalURL, nil
}

// Ping : vérifie que les credentials Nextcloud sont corrects.
// On utilise l'API OCS status qui est un simple GET — supporté par tous les
// reverse proxies (contrairement à PROPFIND qui peut être bloqué par nginx).
// 200 = auth OK, 401 = credentials invalides.
func Ping(baseURL, username, password string) error {
	if baseURL == "" {
		return fmt.Errorf("URL WebDAV manquante")
	}
	base := strings.TrimRight(baseURL, "/")
	// L'endpoint OCS user renvoie un XML avec les infos du user connecté.
	// Requiert une session authentifiée → parfait pour valider les creds.
	u := base + "/ocs/v1.php/cloud/user"
	req, _ := http.NewRequest("GET", u, nil)
	req.SetBasicAuth(username, password)
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("User-Agent", "GoPostTools/3.3")
	c := &http.Client{Timeout: 15 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return nil
	}
	if resp.StatusCode == 401 {
		return fmt.Errorf("401 Unauthorized — vérifie user/password")
	}
	// Fallback : essaie un simple GET authentifié sur la racine files
	// (certains providers désactivent OCS mais gardent /remote.php/dav).
	req2, _ := http.NewRequest("GET", base+"/remote.php/dav/files/"+url.PathEscape(username)+"/", nil)
	req2.SetBasicAuth(username, password)
	req2.Header.Set("User-Agent", "GoPostTools/3.3")
	resp2, err := c.Do(req2)
	if err != nil {
		return fmt.Errorf("HTTP %d sur /ocs et fallback failed: %v", resp.StatusCode, err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode == 200 || resp2.StatusCode == 207 {
		return nil
	}
	if resp2.StatusCode == 401 {
		return fmt.Errorf("401 Unauthorized — vérifie user/password")
	}
	return fmt.Errorf("HTTP %d (OCS) / HTTP %d (fallback dav) — URL racine correcte ?", resp.StatusCode, resp2.StatusCode)
}
