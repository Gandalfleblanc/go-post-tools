package ftpup

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/jlaffaye/ftp"
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

// watchdogFTP ferme la connexion FTP quand le contexte est annulé.
// Retourne une fonction à appeler en defer pour stopper le watchdog.
func watchdogFTP(ctx context.Context, c *ftp.ServerConn) func() {
	if ctx == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = c.Quit()
		case <-done:
		}
	}()
	return func() { close(done) }
}

// UploadFromReader pousse les données du reader vers le FTP sous le nom donné.
// total est utilisé pour le calcul du pourcentage ; peut être 0 si inconnu.
func UploadFromReader(ctx context.Context, host string, port int, user, password, remotePath, remoteName string, r io.Reader, total int64, onProgress func(Progress)) error {
	if host == "" {
		return fmt.Errorf("host FTP manquant")
	}
	if port <= 0 {
		port = 21
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	c, err := ftp.Dial(addr, ftp.DialWithTimeout(15*time.Second))
	if err != nil {
		return fmt.Errorf("connexion FTP: %w", err)
	}
	defer c.Quit()
	stopWatch := watchdogFTP(ctx, c)
	defer stopWatch()
	if err := c.Login(user, password); err != nil {
		return fmt.Errorf("login FTP: %w", err)
	}
	if remotePath != "" && remotePath != "/" {
		if err := c.ChangeDir(remotePath); err != nil {
			return fmt.Errorf("cd %s: %w", remotePath, err)
		}
	}
	now := time.Now()
	body := r
	if onProgress != nil && total > 0 {
		body = &progressReader{r: r, total: total, start: now, lastEmit: now, onProgress: onProgress}
	}
	if err := c.Stor(remoteName, body); err != nil {
		if ctx != nil && ctx.Err() != nil {
			return fmt.Errorf("annulé")
		}
		return fmt.Errorf("stor %s: %w", remoteName, err)
	}
	if onProgress != nil {
		onProgress(Progress{Percent: 100})
	}
	return nil
}

// Upload envoie filePath vers le FTP et retourne le nom distant utilisé.
func Upload(ctx context.Context, host string, port int, user, password, remotePath, filePath string, onProgress func(Progress)) (string, error) {
	if host == "" {
		return "", fmt.Errorf("host FTP manquant")
	}
	if port <= 0 {
		port = 21
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	c, err := ftp.Dial(addr, ftp.DialWithTimeout(15*time.Second))
	if err != nil {
		return "", fmt.Errorf("connexion FTP: %w", err)
	}
	defer c.Quit()
	stopWatch := watchdogFTP(ctx, c)
	defer stopWatch()

	if err := c.Login(user, password); err != nil {
		return "", fmt.Errorf("login FTP: %w", err)
	}

	if remotePath != "" && remotePath != "/" {
		if err := c.ChangeDir(remotePath); err != nil {
			return "", fmt.Errorf("cd %s: %w", remotePath, err)
		}
	}

	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("ouverture %s: %w", filePath, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}

	remoteName := filepath.Base(filePath)
	now := time.Now()
	body := io.Reader(f)
	if onProgress != nil {
		body = &progressReader{r: f, total: info.Size(), start: now, lastEmit: now, onProgress: onProgress}
	}

	if err := c.Stor(remoteName, body); err != nil {
		if ctx != nil && ctx.Err() != nil {
			return "", fmt.Errorf("annulé")
		}
		return "", fmt.Errorf("stor %s: %w", remoteName, err)
	}

	if onProgress != nil {
		onProgress(Progress{Percent: 100})
	}
	return remoteName, nil
}

// Delete supprime un fichier sur le FTP. Retourne nil si fichier supprimé OU
// déjà absent (DELE renvoie 550 sur fichier inexistant — on traite comme succès).
func Delete(host string, port int, user, password, remotePath, remoteName string) error {
	if host == "" {
		return fmt.Errorf("host FTP manquant")
	}
	if port <= 0 {
		port = 21
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	c, err := ftp.Dial(addr, ftp.DialWithTimeout(15*time.Second))
	if err != nil {
		return fmt.Errorf("connexion FTP: %w", err)
	}
	defer c.Quit()

	if err := c.Login(user, password); err != nil {
		return fmt.Errorf("login FTP: %w", err)
	}
	if remotePath != "" && remotePath != "/" {
		if err := c.ChangeDir(remotePath); err != nil {
			return fmt.Errorf("cd %s: %w", remotePath, err)
		}
	}
	if err := c.Delete(remoteName); err != nil {
		// 550 = fichier inexistant → on considère que c'est OK (déjà supprimé)
		es := err.Error()
		if containsAny(es, []string{"550", "not found", "No such"}) {
			return nil
		}
		return fmt.Errorf("DELE %s: %w", remoteName, err)
	}
	return nil
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}
