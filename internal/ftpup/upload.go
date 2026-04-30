package ftpup

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
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

// UploadFolder upload récursivement un dossier local vers FTP.
// Crée le sous-dossier remotePath/folderName puis stor chaque fichier.
// Le progress cumule la taille de tous les fichiers.
// Retourne le nom du dossier remote (= basename du dossier local).
func UploadFolder(ctx context.Context, host string, port int, user, password, remotePath, localFolder string, onProgress func(Progress)) (string, error) {
	if host == "" {
		return "", fmt.Errorf("host FTP manquant")
	}
	if port <= 0 {
		port = 21
	}
	folderName := filepath.Base(localFolder)
	// Walk pour collecter les fichiers et calculer la taille totale
	var files []string
	var totalSize int64
	if err := filepath.Walk(localFolder, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() {
			files = append(files, p)
			totalSize += info.Size()
		}
		return nil
	}); err != nil {
		return "", fmt.Errorf("walk %s: %w", localFolder, err)
	}
	if len(files) == 0 {
		return "", fmt.Errorf("dossier vide: %s", localFolder)
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
	// MakeDir idempotent : tente de créer, ignore si déjà existe
	_ = c.MakeDir(folderName)
	if err := c.ChangeDir(folderName); err != nil {
		return "", fmt.Errorf("cd %s: %w", folderName, err)
	}

	start := time.Now()
	var sentTotal int64
	var lastEmit time.Time
	for _, file := range files {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("annulé")
		default:
		}
		f, err := os.Open(file)
		if err != nil {
			return "", fmt.Errorf("open %s: %w", file, err)
		}
		stat, err := f.Stat()
		if err != nil {
			f.Close()
			return "", err
		}
		fileSize := stat.Size()
		rel, err := filepath.Rel(localFolder, file)
		if err != nil {
			f.Close()
			return "", err
		}
		// Si fichier dans un sous-dossier, on crée la hiérarchie
		if dir := filepath.Dir(rel); dir != "" && dir != "." {
			parts := strings.Split(filepath.ToSlash(dir), "/")
			for _, part := range parts {
				_ = c.MakeDir(part)
				if err := c.ChangeDir(part); err != nil {
					f.Close()
					return "", fmt.Errorf("cd %s: %w", part, err)
				}
			}
		}
		// Wrap reader pour progrès cumulé
		baseSent := sentTotal
		body := io.Reader(f)
		if onProgress != nil {
			body = &progressReader{
				r:     f,
				total: fileSize,
				start: start,
				onProgress: func(p Progress) {
					localRead := int64(p.Percent / 100 * float64(fileSize))
					cumul := baseSent + localRead
					if time.Since(lastEmit) < 250*time.Millisecond {
						return
					}
					lastEmit = time.Now()
					elapsed := time.Since(start).Seconds()
					speed := 0.0
					if elapsed > 0.1 {
						speed = float64(cumul) / elapsed / 1024 / 1024
					}
					cumPct := math.Min(float64(cumul)/float64(totalSize)*100, 99)
					onProgress(Progress{Percent: cumPct, SpeedMB: speed})
				},
			}
		}
		if err := c.Stor(filepath.Base(rel), body); err != nil {
			f.Close()
			if ctx != nil && ctx.Err() != nil {
				return "", fmt.Errorf("annulé")
			}
			return "", fmt.Errorf("stor %s: %w", rel, err)
		}
		f.Close()
		// Remonte au niveau du folderName racine pour le prochain fichier
		if dir := filepath.Dir(rel); dir != "" && dir != "." {
			parts := strings.Split(filepath.ToSlash(dir), "/")
			for range parts {
				if err := c.ChangeDirToParent(); err != nil {
					return "", fmt.Errorf("cdup: %w", err)
				}
			}
		}
		sentTotal += fileSize
	}
	if onProgress != nil {
		onProgress(Progress{Percent: 100})
	}
	return folderName, nil
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
