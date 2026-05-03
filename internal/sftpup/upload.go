// Package sftpup uploade des fichiers via SFTP (SSH File Transfer Protocol, port 22).
//
// Utilisé en remplacement de ftpup quand l'ISP throttle FTP (port 21) mais pas
// SSH (port 22). Même seedbox (ex: ma-seedbox.me), mêmes credentials, juste un
// port + protocole différent. ISP voit que du trafic SSH chiffré, peut pas
// throttle spécifiquement.
//
// API mirror de ftpup pour drop-in replacement :
//   - Upload(ctx, host, port, user, password, remotePath, filePath, onProgress)
//   - UploadFolder(ctx, host, port, user, password, remotePath, localFolder, onProgress)
package sftpup

import (
	"context"
	"fmt"
	"math"
	"net"
	"os"
	"path"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Progress struct {
	Percent float64 `json:"percent"`
	SpeedMB float64 `json:"speed_mb"`
}

// readerAtCounter wrappe un *os.File en exposant Read + ReadAt + Size().
// La méthode Size() est CRITIQUE : sans elle pkg/sftp ne sait pas combien
// d'octets il va lire et fait fallback sur le chemin séquentiel = ~0.5 Mb/s
// au lieu du concurrent writes (jusqu'à 64 packets en vol = ~40 Mb/s).
// Les bytes lus sont comptés via atomic (ReadAt peut être appelé depuis
// plusieurs goroutines en parallèle par pkg/sftp).
type readerAtCounter struct {
	f     *os.File
	size  int64
	ctx   context.Context
	count int64 // atomic
}

func (r *readerAtCounter) ReadAt(p []byte, off int64) (n int, err error) {
	if r.ctx != nil {
		if cerr := r.ctx.Err(); cerr != nil {
			return 0, cerr
		}
	}
	n, err = r.f.ReadAt(p, off)
	if n > 0 {
		atomic.AddInt64(&r.count, int64(n))
	}
	return
}

func (r *readerAtCounter) Read(p []byte) (n int, err error) {
	if r.ctx != nil {
		if cerr := r.ctx.Err(); cerr != nil {
			return 0, cerr
		}
	}
	n, err = r.f.Read(p)
	if n > 0 {
		atomic.AddInt64(&r.count, int64(n))
	}
	return
}

// Size : exposé pour que pkg/sftp détecte la taille et active concurrent writes.
func (r *readerAtCounter) Size() int64 { return r.size }

// dialSSH établit une session SSH avec keepalive 30s.
// Ciphers/MACs explicites : Go's default AES-CTR est ~10x + lent que chacha20-poly1305
// ou aes128-gcm. Sans cette override on est CPU-bound à ~0.5-3 MB/s.
func dialSSH(host string, port int, user, password string) (*ssh.Client, error) {
	if port <= 0 {
		port = 22
	}
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // les seedbox ont rarement un cert valide
		Timeout:         15 * time.Second,
		Config: ssh.Config{
			Ciphers: []string{
				"chacha20-poly1305@openssh.com", // le + rapide en Go pur (pas d'AES-NI requis)
				"aes128-gcm@openssh.com",        // fast si CPU AES-NI
				"aes256-gcm@openssh.com",
				"aes128-ctr",                     // fallback compat
				"aes192-ctr",
				"aes256-ctr",
			},
			MACs: []string{
				"hmac-sha2-256-etm@openssh.com",
				"hmac-sha2-512-etm@openssh.com",
				"hmac-sha2-256",
				"hmac-sha2-512",
			},
		},
	}
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial tcp: %w", err)
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ssh handshake: %w", err)
	}
	return ssh.NewClient(c, chans, reqs), nil
}

// Upload pousse filePath vers remote sftp avec auto-resume + cancel.
// Retourne le nom remote (= basename de filePath).
func Upload(ctx context.Context, host string, port int, user, password, remotePath, filePath string, onProgress func(Progress)) (string, error) {
	if host == "" {
		return "", fmt.Errorf("host SFTP manquant")
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
	totalSize := info.Size()
	remoteName := filepath.Base(filePath)
	if remotePath == "" {
		remotePath = "/"
	}
	remoteFull := path.Join(remotePath, remoteName)

	var sentBytes int64
	maxRetries := 5
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if ctx != nil && ctx.Err() != nil {
			return "", fmt.Errorf("annulé")
		}
		sshClient, err := dialSSH(host, port, user, password)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
			continue
		}
		// Options perf : multiplexe les Write calls en plusieurs paquets SFTP en
		// parallèle. Sans ça, 1 paquet 32KB par RTT = débit collapsé sur Hetzner.
		// On garde MaxPacket par défaut (32KB) — pas tous les serveurs supportent
		// les paquets >32KB et ça causait des uploads "fantôme" (0 byte).
		sftpClient, err := sftp.NewClient(sshClient,
			sftp.MaxConcurrentRequestsPerFile(64),
			sftp.UseConcurrentWrites(true),
		)
		if err != nil {
			sshClient.Close()
			lastErr = fmt.Errorf("sftp client: %w", err)
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
			continue
		}
		// Watchdog : ferme la session si ctx cancelled
		watchDone := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				_ = sftpClient.Close()
				_ = sshClient.Close()
			case <-watchDone:
			}
		}()
		stopWatch := func() { close(watchDone) }

		// O_TRUNC à chaque tentative : le resume O_APPEND est incompatible avec
		// les concurrent writes (les WriteAt à des offsets parallèles ne marchent
		// qu'avec un fichier ouvert en O_WRONLY|O_CREATE|O_TRUNC).
		flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		rfile, err := sftpClient.OpenFile(remoteFull, flag)
		if err != nil {
			stopWatch()
			sftpClient.Close()
			sshClient.Close()
			lastErr = fmt.Errorf("open remote %s: %w", remoteFull, err)
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
			continue
		}

		// Seek à 0 : on retransmet tout le fichier à chaque tentative (resume drop).
		if _, err := f.Seek(0, 0); err != nil {
			stopWatch()
			rfile.Close()
			sftpClient.Close()
			sshClient.Close()
			return "", fmt.Errorf("seek file: %w", err)
		}

		// Source = wrapper ReaderAt+Reader+Size avec atomic counter. Size() est
		// requis pour activer le chemin "concurrent writes" de pkg/sftp.
		src := &readerAtCounter{f: f, size: totalSize, ctx: ctx}
		sentBytes = 0

		// Goroutine progress : poll l'atomic counter toutes les 250ms et émet.
		// Découplée de l'upload pour ne pas perturber les Read concurrents.
		progDone := make(chan struct{})
		startedAt := time.Now()
		if onProgress != nil {
			go func() {
				ticker := time.NewTicker(250 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-progDone:
						return
					case <-ticker.C:
						read := atomic.LoadInt64(&src.count)
						elapsed := time.Since(startedAt).Seconds()
						speed := 0.0
						if elapsed > 0.1 {
							speed = float64(read) / elapsed / 1024 / 1024
						}
						pct := float64(sentBytes+read) / float64(totalSize) * 100
						onProgress(Progress{Percent: math.Min(pct, 99), SpeedMB: speed})
					}
				}
			}()
		}

		n, copyErr := rfile.ReadFrom(src)
		close(progDone)
		sentBytes += n
		rfile.Close()
		stopWatch()
		sftpClient.Close()
		sshClient.Close()

		if copyErr == nil {
			if onProgress != nil {
				onProgress(Progress{Percent: 100})
			}
			return remoteName, nil
		}
		if ctx != nil && ctx.Err() != nil {
			return "", fmt.Errorf("annulé")
		}
		lastErr = fmt.Errorf("sftp copy %s (attempt %d/%d, sent %d/%d): %w",
			remoteName, attempt+1, maxRetries, sentBytes, totalSize, copyErr)
		time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
	}
	return "", lastErr
}

// UploadFolder upload récursif d'un dossier local vers remote SFTP.
// Crée remotePath/folderName/ et envoie chaque fichier.
func UploadFolder(ctx context.Context, host string, port int, user, password, remotePath, localFolder string, onProgress func(Progress)) (string, error) {
	folderName := filepath.Base(localFolder)
	// Walk pour collecter fichiers + total size
	var files []string
	var totalSize int64
	if err := filepath.Walk(localFolder, func(p string, info os.FileInfo, e error) error {
		if e != nil {
			return e
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

	if ctx != nil && ctx.Err() != nil {
		return "", fmt.Errorf("annulé")
	}
	sshClient, err := dialSSH(host, port, user, password)
	if err != nil {
		return "", err
	}
	defer sshClient.Close()
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return "", fmt.Errorf("sftp client: %w", err)
	}
	defer sftpClient.Close()

	if remotePath == "" {
		remotePath = "/"
	}
	remoteFolder := path.Join(remotePath, folderName)
	_ = sftpClient.MkdirAll(remoteFolder)

	start := time.Now()
	var sentTotal int64
	for _, file := range files {
		if ctx != nil && ctx.Err() != nil {
			return "", fmt.Errorf("annulé")
		}
		f, err := os.Open(file)
		if err != nil {
			return "", fmt.Errorf("open %s: %w", file, err)
		}
		stat, _ := f.Stat()
		fileSize := stat.Size()
		rel, _ := filepath.Rel(localFolder, file)
		// Crée sous-dossiers si nécessaire
		if dir := path.Dir(filepath.ToSlash(rel)); dir != "" && dir != "." {
			_ = sftpClient.MkdirAll(path.Join(remoteFolder, dir))
		}
		remoteFull := path.Join(remoteFolder, filepath.ToSlash(rel))
		rfile, err := sftpClient.Create(remoteFull)
		if err != nil {
			f.Close()
			return "", fmt.Errorf("create remote %s: %w", remoteFull, err)
		}
		// Source ReaderAt+Reader+Size → active concurrent writes pkg/sftp.
		baseSent := sentTotal
		src := &readerAtCounter{f: f, size: fileSize, ctx: ctx}

		// Goroutine progress : agrège les bytes du fichier courant + sentTotal.
		progDone := make(chan struct{})
		if onProgress != nil {
			go func() {
				ticker := time.NewTicker(250 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-progDone:
						return
					case <-ticker.C:
						read := atomic.LoadInt64(&src.count)
						cumul := baseSent + read
						elapsed := time.Since(start).Seconds()
						speed := 0.0
						if elapsed > 0.1 {
							speed = float64(cumul) / elapsed / 1024 / 1024
						}
						cumPct := math.Min(float64(cumul)/float64(totalSize)*100, 99)
						onProgress(Progress{Percent: cumPct, SpeedMB: speed})
					}
				}
			}()
		}

		_, copyErr := rfile.ReadFrom(src)
		close(progDone)
		rfile.Close()
		f.Close()
		if copyErr != nil {
			if ctx != nil && ctx.Err() != nil {
				return "", fmt.Errorf("annulé")
			}
			return "", fmt.Errorf("copy %s: %w", rel, copyErr)
		}
		sentTotal += fileSize
	}
	if onProgress != nil {
		onProgress(Progress{Percent: 100})
	}
	return folderName, nil
}

// Ping teste la connexion SSH (utilisé par tester.TestSFTP).
func Ping(host string, port int, user, password string) error {
	c, err := dialSSH(host, port, user, password)
	if err != nil {
		return err
	}
	defer c.Close()
	s, err := sftp.NewClient(c)
	if err != nil {
		return fmt.Errorf("sftp: %w", err)
	}
	defer s.Close()
	if _, err := s.ReadDir("/"); err != nil {
		return fmt.Errorf("readdir: %w", err)
	}
	return nil
}
