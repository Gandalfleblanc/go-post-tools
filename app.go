package main

import (
	"context"
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"sort"
	"time"

	"os"
	"path/filepath"
	"strings"

	"sync"

	"go-post-tools/api"
	"go-post-tools/internal/config"
	"go-post-tools/internal/downloader"
	"go-post-tools/internal/ftpup"
	"go-post-tools/internal/history"
	"go-post-tools/internal/lihdl"
	"go-post-tools/internal/nexum"
	"go-post-tools/internal/nyuu"
	"go-post-tools/internal/rutorrent"
	"go-post-tools/internal/mediasearch"

	"github.com/anacrolix/torrent/metainfo"
	"go-post-tools/internal/parpar"
	"go-post-tools/internal/parser"
	"go-post-tools/internal/qbittorrent"
	"go-post-tools/internal/seedbox"
	"go-post-tools/internal/tester"
	"go-post-tools/internal/tmdb"
	"go-post-tools/internal/torrent"
	"go-post-tools/internal/uploader"
	"go-post-tools/internal/watcher"

	"github.com/gen2brain/beeep"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/bcrypt"
)

const Version = "5.2.1"

type App struct {
	ctx         context.Context
	client      *api.Client
	cfg         *config.Config
	cancelMu    sync.Mutex
	cancelled   bool
	workCtx     context.Context
	workCancel  context.CancelFunc
	watch       *watcher.Watcher
	watchMu     sync.Mutex
	hist        *history.Store
	hostCancels map[string]context.CancelFunc // DDL : annulation par host
	hostMu      sync.Mutex
	currentUser *AuthResult // session en mémoire (login bcrypt) — nil = non connecté
	sessionMu   sync.Mutex
}

// GetVersion retourne la version de l'application.
func (a *App) GetVersion() string { return Version }

type UpdateInfo struct {
	Available bool   `json:"available"`
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	URL       string `json:"url"`
}

// CheckForUpdate interroge GitHub Releases et compare avec la version locale.
func (a *App) CheckForUpdate() (*UpdateInfo, error) {
	info := &UpdateInfo{Current: Version}
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/Gandalfleblanc/Go-Post-Tools/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return info, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return info, nil
	}
	var data struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return info, err
	}
	latest := strings.TrimPrefix(data.TagName, "v")
	info.Latest = latest
	info.URL = data.HTMLURL
	info.Available = latest != "" && latest != Version
	return info, nil
}

// assetNameForPlatform retourne le nom d'asset GitHub pour la plateforme courante.
func assetNameForPlatform() string {
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return "Go-Post-Tools-macos-arm64.zip"
		}
		return "Go-Post-Tools-macos-amd64.zip"
	case "linux":
		return "Go-Post-Tools-linux-amd64.tar.gz"
	case "windows":
		return "Go-Post-Tools-windows-amd64.zip"
	}
	return ""
}

// DownloadUpdate télécharge l'asset de la dernière release dans le dossier Téléchargements
// et ouvre le Finder/Explorer dessus. Retourne le chemin local du fichier téléchargé.
func (a *App) DownloadUpdate() (string, error) {
	assetName := assetNameForPlatform()
	if assetName == "" {
		return "", fmt.Errorf("plateforme non supportée pour la mise à jour auto")
	}
	emit := func(stage, msg string, pct float64) {
		wailsruntime.EventsEmit(a.ctx, "update:progress", map[string]interface{}{"stage": stage, "msg": msg, "percent": pct})
	}

	emit("meta", "Récupération de la release…", 0)
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/Gandalfleblanc/Go-Post-Tools/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	c := &http.Client{Timeout: 15 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var data struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}
	var url string
	var total int64
	for _, ast := range data.Assets {
		if ast.Name == assetName {
			url = ast.BrowserDownloadURL
			total = ast.Size
			break
		}
	}
	if url == "" {
		return "", fmt.Errorf("asset %s introuvable dans la release %s", assetName, data.TagName)
	}

	home, _ := os.UserHomeDir()
	downloads := filepath.Join(home, "Downloads")
	_ = os.MkdirAll(downloads, 0755)
	outPath := filepath.Join(downloads, assetName)

	emit("download", fmt.Sprintf("Téléchargement de %s…", assetName), 0)
	dlReq, _ := http.NewRequest("GET", url, nil)
	dlClient := &http.Client{Timeout: 10 * time.Minute}
	dlResp, err := dlClient.Do(dlReq)
	if err != nil {
		return "", err
	}
	defer dlResp.Body.Close()

	out, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	buf := make([]byte, 256*1024)
	var read int64
	lastEmit := time.Now()
	for {
		n, er := dlResp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return "", werr
			}
			read += int64(n)
			if total > 0 && time.Since(lastEmit) > 200*time.Millisecond {
				emit("download", fmt.Sprintf("%.1f / %.1f MB", float64(read)/1024/1024, float64(total)/1024/1024), float64(read)/float64(total)*100)
				lastEmit = time.Now()
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			return "", er
		}
	}
	emit("done", "Téléchargement terminé", 100)

	// Auto-install : extrait l'archive, lance un helper qui remplace l'app
	// et relance la nouvelle version après la fermeture de l'app courante.
	emit("install", "Installation de la mise à jour…", 100)
	if err := a.applyUpdate(outPath); err != nil {
		// Fallback : on ouvre juste le dossier si l'auto-install échoue
		switch runtime.GOOS {
		case "darwin":
			_ = exec.Command("open", "-R", outPath).Run()
		case "linux":
			_ = exec.Command("xdg-open", downloads).Run()
		case "windows":
			_ = exec.Command("explorer", "/select,", outPath).Run()
		}
		return outPath, fmt.Errorf("auto-install échoué (%w) — fichier dans ~/Downloads, installe manuellement", err)
	}
	// applyUpdate a démarré le helper et va quitter l'app. L'UI peut afficher
	// "Redémarrage…" — l'app se ferme dans ~1s.
	return outPath, nil
}

// applyUpdate extrait l'archive téléchargée et lance un helper script qui remplace
// l'app courante puis relance la nouvelle version. L'app elle-même quitte ensuite.
func (a *App) applyUpdate(archivePath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// Résout les symlinks pour avoir le vrai chemin (ex: /Applications/X.app/Contents/MacOS/X)
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	tmp, err := os.MkdirTemp("", "gpt-update-")
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "darwin":
		// Archive = .zip contenant "Go Post Tools.app"
		if err := unzipTo(archivePath, tmp); err != nil {
			return fmt.Errorf("unzip: %w", err)
		}
		newApp := filepath.Join(tmp, "Go Post Tools.app")
		// Le binaire courant est .../Go Post Tools.app/Contents/MacOS/Go Post Tools
		// L'app bundle est donc 3 dossiers plus haut.
		appBundle := exe
		for i := 0; i < 3; i++ {
			appBundle = filepath.Dir(appBundle)
		}
		helper := filepath.Join(tmp, "install.sh")
		script := fmt.Sprintf(`#!/bin/bash
sleep 2
rm -rf %q
cp -R %q %q
xattr -cr %q 2>/dev/null
open %q
`, appBundle, newApp, appBundle, appBundle, appBundle)
		if err := os.WriteFile(helper, []byte(script), 0755); err != nil {
			return err
		}
		cmd := exec.Command("/bin/bash", helper)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			return err
		}

	case "linux":
		// Archive = .tar.gz contenant le binaire "Go Post Tools"
		if err := untarGzTo(archivePath, tmp); err != nil {
			return fmt.Errorf("untar: %w", err)
		}
		newBin := filepath.Join(tmp, "Go Post Tools")
		helper := filepath.Join(tmp, "install.sh")
		script := fmt.Sprintf(`#!/bin/bash
sleep 2
cp -f %q %q
chmod +x %q
nohup %q >/dev/null 2>&1 &
`, newBin, exe, exe, exe)
		if err := os.WriteFile(helper, []byte(script), 0755); err != nil {
			return err
		}
		if err := exec.Command("/bin/bash", helper).Start(); err != nil {
			return err
		}

	case "windows":
		// Archive = .zip contenant "Go Post Tools.exe" + (optionnel) RTF
		if err := unzipTo(archivePath, tmp); err != nil {
			return fmt.Errorf("unzip: %w", err)
		}
		newExe := filepath.Join(tmp, "Go Post Tools.exe")
		helper := filepath.Join(tmp, "install.bat")
		// timeout + move + relaunch + suppression du bat
		script := fmt.Sprintf(`@echo off
timeout /t 2 /nobreak > NUL
move /Y "%s" "%s"
start "" "%s"
del "%%~f0"
`, newExe, exe, exe)
		if err := os.WriteFile(helper, []byte(script), 0755); err != nil {
			return err
		}
		cmd := exec.Command("cmd", "/C", "start", "/B", helper)
		if err := cmd.Start(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("auto-install non supporté sur %s", runtime.GOOS)
	}

	// Quitte l'app après un court délai pour que l'UI puisse afficher le message final
	go func() {
		time.Sleep(500 * time.Millisecond)
		wailsruntime.Quit(a.ctx)
	}()
	return nil
}

// unzipTo extrait un .zip dans un dossier destination.
func unzipTo(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		target := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("chemin zip suspect: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, f.Mode())
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// untarGzTo extrait un .tar.gz dans un dossier destination.
func untarGzTo(tgzPath, dest string) error {
	f, err := os.Open(tgzPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, h.Name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("chemin tar suspect: %s", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(target, os.FileMode(h.Mode))
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(h.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

// StartWatchFolder démarre la surveillance du dossier configuré.
func (a *App) StartWatchFolder(folder string) error {
	a.watchMu.Lock()
	defer a.watchMu.Unlock()
	if a.watch != nil {
		a.watch.Stop()
		a.watch = nil
	}
	if folder == "" {
		folder = a.cfg.WatchFolder
	}
	if folder == "" {
		return fmt.Errorf("dossier à surveiller manquant")
	}
	w := watcher.New(folder, func(path string) {
		wailsruntime.EventsEmit(a.ctx, "watch:newfile", path)
	})
	if err := w.Start(); err != nil {
		return err
	}
	a.watch = w
	wailsruntime.EventsEmit(a.ctx, "watch:status", map[string]interface{}{"running": true, "folder": folder})
	return nil
}

// StopWatchFolder arrête la surveillance.
func (a *App) StopWatchFolder() {
	a.watchMu.Lock()
	defer a.watchMu.Unlock()
	if a.watch != nil {
		a.watch.Stop()
		a.watch = nil
	}
	wailsruntime.EventsEmit(a.ctx, "watch:status", map[string]interface{}{"running": false})
}

// IsWatching retourne si la surveillance est active.
func (a *App) IsWatching() bool {
	a.watchMu.Lock()
	defer a.watchMu.Unlock()
	return a.watch != nil && a.watch.IsRunning()
}

// resetCancellation (ré)initialise le contexte de travail uniquement s'il est
// déjà expiré ou non défini. Permet aux workflows parallèles (DDL+NZB+Torrent)
// de partager le même contexte sans s'annuler mutuellement.
func (a *App) resetCancellation() {
	a.cancelMu.Lock()
	needsReset := a.workCtx == nil || a.workCtx.Err() != nil || a.cancelled
	if needsReset {
		if a.workCancel != nil {
			a.workCancel()
		}
		a.workCtx, a.workCancel = context.WithCancel(context.Background())
		a.cancelled = false
	}
	ctx := a.workCtx
	a.cancelMu.Unlock()
	a.client.SetContext(ctx)
}

// CancelAllWorkflows demande l'arrêt des workflows en cours (annulation immédiate).
func (a *App) CancelAllWorkflows() {
	a.cancelMu.Lock()
	a.cancelled = true
	if a.workCancel != nil {
		a.workCancel()
	}
	a.cancelMu.Unlock()
	wailsruntime.EventsEmit(a.ctx, "post:cancelled", true)
}

func (a *App) isCancelled() bool {
	a.cancelMu.Lock()
	defer a.cancelMu.Unlock()
	return a.cancelled
}

// workContext retourne le contexte courant pour les workflows (ou context.Background si jamais démarré).
func (a *App) workContext() context.Context {
	a.cancelMu.Lock()
	defer a.cancelMu.Unlock()
	if a.workCtx == nil {
		return context.Background()
	}
	return a.workCtx
}

// effectiveHydrackerURL retourne l'URL API à utiliser.
// Si un DefaultHydrackerBaseURL est injecté au build (secret CI), il prime sur
// la config user. Sinon on utilise ce que l'user a renseigné dans Réglages.
func effectiveHydrackerURL(cfg *config.Config) string {
	if config.DefaultHydrackerBaseURL != "" {
		return config.DefaultHydrackerBaseURL
	}
	return cfg.HydrackerBaseURL
}

// IsHydrackerURLManaged indique si l'URL API est verrouillée au build (non modifiable par user).
func (a *App) IsHydrackerURLManaged() bool {
	return config.DefaultHydrackerBaseURL != ""
}

// GetEffectiveHydrackerURL retourne l'URL utilisée en ce moment (build-time > user config).
func (a *App) GetEffectiveHydrackerURL() string {
	return effectiveHydrackerURL(a.cfg)
}

func NewApp() *App {
	cfg := config.Load()
	hist, _ := history.Open()
	return &App{
		client:      api.NewClient(cfg.HydrackerToken, effectiveHydrackerURL(cfg)),
		cfg:         cfg,
		hist:        hist,
		hostCancels: map[string]context.CancelFunc{},
	}
}

// CancelDDLHost annule l'upload en cours sur un host DDL spécifique (l'autre continue).
func (a *App) CancelDDLHost(host string) {
	a.hostMu.Lock()
	defer a.hostMu.Unlock()
	if cancel, ok := a.hostCancels[host]; ok {
		cancel()
		delete(a.hostCancels, host)
		wailsruntime.EventsEmit(a.ctx, "ddl:host-skipped", host)
	}
}

// SkipCurrentEpisode annule les workflows de l'épisode en cours mais laisse la queue continuer.
func (a *App) SkipCurrentEpisode() {
	a.cancelMu.Lock()
	if a.workCancel != nil {
		a.workCancel()
	}
	// ne PAS set cancelled=true — on veut que la queue continue à l'ép suivant
	a.cancelMu.Unlock()
	wailsruntime.EventsEmit(a.ctx, "post:skipped", true)
}

// --- Historique ---

func (a *App) HistoryList(filterType, query string, titleID, limit int) ([]history.Entry, error) {
	if a.hist == nil {
		return nil, fmt.Errorf("historique indisponible")
	}
	if limit <= 0 {
		limit = 500
	}
	return a.hist.List(filterType, query, titleID, limit)
}

func (a *App) HistoryDelete(id int64) error {
	if a.hist == nil {
		return fmt.Errorf("historique indisponible")
	}
	return a.hist.Delete(id)
}

func (a *App) HistoryStats() (map[string]int, error) {
	if a.hist == nil {
		return nil, fmt.Errorf("historique indisponible")
	}
	return a.hist.Stats()
}

// Notify affiche une notification native (macOS/Linux/Windows).
func (a *App) Notify(title, message string) {
	_ = beeep.Notify(title, message, "")
}

// --- Protection mot de passe section LiHDL (UI gatekeeping) ---

func hashPassword(p string) string {
	h := sha256.Sum256([]byte(p))
	return hex.EncodeToString(h[:])
}

// activeLihdlHash retourne le hash qui protège la section.
// Priorité : build-time (injecté via ldflags depuis secret GitHub) > config user > vide (pas de protection).
func (a *App) activeLihdlHash() string {
	if config.DefaultLihdlUnlockHash != "" {
		return config.DefaultLihdlUnlockHash
	}
	return a.cfg.LihdlSettingsPasswordHash
}

// HasLihdlSettingsPassword retourne true si un mot de passe est défini
// (soit injecté au build via secret GitHub, soit défini par l'utilisateur).
func (a *App) HasLihdlSettingsPassword() bool {
	return a.activeLihdlHash() != ""
}

// IsLihdlPasswordManaged indique si le mdp est imposé au build (non modifiable par user).
func (a *App) IsLihdlPasswordManaged() bool {
	return config.DefaultLihdlUnlockHash != ""
}

// SetLihdlSettingsPassword définit un mot de passe user-side (hash SHA-256 stocké).
// Désactivé si un hash est injecté au build.
func (a *App) SetLihdlSettingsPassword(currentPassword, newPassword string) error {
	if config.DefaultLihdlUnlockHash != "" {
		return fmt.Errorf("mot de passe géré par le build, modification impossible")
	}
	if newPassword == "" {
		return fmt.Errorf("mot de passe vide")
	}
	if a.cfg.LihdlSettingsPasswordHash != "" {
		if hashPassword(currentPassword) != a.cfg.LihdlSettingsPasswordHash {
			return fmt.Errorf("mot de passe actuel incorrect")
		}
	}
	a.cfg.LihdlSettingsPasswordHash = hashPassword(newPassword)
	return config.Save(a.cfg)
}

// VerifyLihdlSettingsPassword compare avec le hash actif (build-time ou user).
func (a *App) VerifyLihdlSettingsPassword(password string) bool {
	h := a.activeLihdlHash()
	if h == "" {
		return true
	}
	return hashPassword(password) == h
}

// ClearLihdlSettingsPassword retire la protection user-side (désactivé si build-time).
func (a *App) ClearLihdlSettingsPassword(currentPassword string) error {
	if config.DefaultLihdlUnlockHash != "" {
		return fmt.Errorf("mot de passe géré par le build, suppression impossible")
	}
	if !a.VerifyLihdlSettingsPassword(currentPassword) {
		return fmt.Errorf("mot de passe incorrect")
	}
	a.cfg.LihdlSettingsPasswordHash = ""
	return config.Save(a.cfg)
}

// --- Protection des sections SEEDBOX/FTP (mdp partagé entre admins) ---
// Priorité : build-time (DefaultSeedboxUnlockHash, secret GitHub) > config user.
// Si build-time non-vide, le mdp est imposé et non modifiable par l'user.

func (a *App) activeSeedboxHash() string {
	if config.DefaultSeedboxUnlockHash != "" {
		return config.DefaultSeedboxUnlockHash
	}
	return a.cfg.SeedboxSettingsPasswordHash
}

func (a *App) HasSeedboxSettingsPassword() bool {
	return a.activeSeedboxHash() != ""
}

func (a *App) IsSeedboxPasswordManaged() bool {
	return config.DefaultSeedboxUnlockHash != ""
}

func (a *App) SetSeedboxSettingsPassword(currentPassword, newPassword string) error {
	if config.DefaultSeedboxUnlockHash != "" {
		return fmt.Errorf("mot de passe géré par le build, modification impossible")
	}
	if newPassword == "" {
		return fmt.Errorf("mot de passe vide")
	}
	if a.cfg.SeedboxSettingsPasswordHash != "" {
		if hashPassword(currentPassword) != a.cfg.SeedboxSettingsPasswordHash {
			return fmt.Errorf("mot de passe actuel incorrect")
		}
	}
	a.cfg.SeedboxSettingsPasswordHash = hashPassword(newPassword)
	return config.Save(a.cfg)
}

func (a *App) VerifySeedboxSettingsPassword(password string) bool {
	h := a.activeSeedboxHash()
	if h == "" {
		return true
	}
	if hashPassword(password) == h {
		// Vérification OK → l'user est admin. On persiste le flag pour que
		// l'option Torrent ADMIN s'affiche dans HydrackerTab sans demander
		// de mdp à nouveau (un admin reste admin pour toujours).
		if !a.cfg.TorrentAdminAcknowledged {
			a.cfg.TorrentAdminAcknowledged = true
			_ = config.Save(a.cfg)
		}
		return true
	}
	return false
}

func (a *App) ClearSeedboxSettingsPassword(currentPassword string) error {
	if config.DefaultSeedboxUnlockHash != "" {
		return fmt.Errorf("mot de passe géré par le build, suppression impossible")
	}
	if !a.VerifySeedboxSettingsPassword(currentPassword) {
		return fmt.Errorf("mot de passe incorrect")
	}
	a.cfg.SeedboxSettingsPasswordHash = ""
	return config.Save(a.cfg)
}

// IsTorrentAdminAcknowledged retourne true si l'user a déjà entré le mdp une fois.
func (a *App) IsTorrentAdminAcknowledged() bool {
	return a.cfg.TorrentAdminAcknowledged
}

// AcknowledgeTorrentAdmin vérifie le mdp partagé et persiste l'acknowledge.
// Retourne nil si OK, erreur si le mdp est incorrect.
func (a *App) AcknowledgeTorrentAdmin(password string) error {
	if !a.VerifySeedboxSettingsPassword(password) {
		return fmt.Errorf("mot de passe incorrect")
	}
	a.cfg.TorrentAdminAcknowledged = true
	return config.Save(a.cfg)
}

// recordHistory enregistre un post dans la base (best-effort, ignore les erreurs).
func (a *App) recordHistory(e history.Entry) {
	if a.hist == nil {
		return
	}
	_ = a.hist.Add(e)
}

// qualiteName retourne le nom humain d'un id qualité (via /meta/quals, best-effort).
func (a *App) qualiteName(id int) string {
	quals, err := a.client.GetQualities()
	if err != nil {
		return ""
	}
	for _, q := range quals {
		if q.ID == id {
			return q.Name
		}
	}
	return ""
}

// titleName récupère le nom du titre depuis son ID (best-effort).
func (a *App) titleName(id int) string {
	t, err := a.client.GetTitle(id, 0, 0, false)
	if err != nil || t == nil {
		return ""
	}
	return t.Name
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// Route les logs du client API vers l'onglet Log API (event api:log).
	a.client.OnRequestLog = func(entry api.APILogEntry) {
		wailsruntime.EventsEmit(a.ctx, "api:log", entry)
	}
	// Applique le proxy HTTP/HTTPS configuré. http.DefaultTransport utilise
	// http.ProxyFromEnvironment qui lit ces env vars à chaque requête, donc
	// les mises à jour via SaveConfig sont prises en compte dynamiquement.
	applyProxy(a.cfg.ProxyURL)
}

// applyProxy set les env vars HTTP_PROXY + HTTPS_PROXY. Passer "" efface.
func applyProxy(proxyURL string) {
	if proxyURL == "" {
		os.Unsetenv("HTTP_PROXY")
		os.Unsetenv("HTTPS_PROXY")
		os.Unsetenv("http_proxy")
		os.Unsetenv("https_proxy")
		return
	}
	os.Setenv("HTTP_PROXY", proxyURL)
	os.Setenv("HTTPS_PROXY", proxyURL)
	os.Setenv("http_proxy", proxyURL)
	os.Setenv("https_proxy", proxyURL)
}

// --- Config ---

func (a *App) GetConfig() *config.Config {
	return a.cfg
}

func (a *App) SaveConfig(cfg config.Config) error {
	a.cfg = &cfg
	a.client.SetToken(cfg.HydrackerToken)
	a.client.SetBaseURL(effectiveHydrackerURL(&cfg))
	applyProxy(cfg.ProxyURL)
	return config.Save(&cfg)
}

// --- Tests de connexion (reçoivent les valeurs directement du frontend) ---

func (a *App) TestLihdl(baseURL, user, password string) tester.Result {
	return tester.TestLihdl(baseURL, user, password)
}

func (a *App) TestHydracker(baseURL, token string) tester.Result {
	return tester.TestHydracker(baseURL, token)
}

func (a *App) TestTMDB(apiKey string) tester.Result {
	return tester.TestTMDB(apiKey)
}

func (a *App) TestOneFichier(apiKey string) tester.Result {
	return tester.TestOneFichier(apiKey)
}

func (a *App) TestSendCm(apiKey string) tester.Result {
	return tester.TestSendCm(apiKey)
}

func (a *App) TestFTP(host string, port int, user, password string) tester.Result {
	return tester.TestFTP(host, port, user, password)
}

func (a *App) TestSeedbox(url, user, password string) tester.Result {
	return tester.TestSeedbox(url, user, password)
}

func (a *App) TestQBit(url, user, password string) tester.Result {
	return tester.TestQBit(url, user, password)
}

func (a *App) TestModSeedbox(url, user, password string) tester.Result {
	return tester.TestModSeedbox(url, user, password)
}

func (a *App) TestUsenet(host string, port int) tester.Result {
	return tester.TestUsenet(host, port)
}

// --- TMDB (via proxytmdb configurable, default https://tmdb.uklm.xyz) ---

// tmdbClient retourne un client TMDB qui respecte la config user (proxy URL).
func (a *App) tmdbClient() *tmdb.Client {
	return tmdb.NewClientWithBase(a.cfg.TMDBProxyURL)
}

func (a *App) TMDBSearch(query string) ([]tmdb.Movie, error) {
	return a.tmdbClient().Search(query)
}

func (a *App) TMDBGetByID(id int, mediaType string) (*tmdb.Movie, error) {
	return a.tmdbClient().GetByID(id, mediaType)
}

// TMDBGetByImdbID : lookup direct via IMDb ID (ex: "tt0120855"). Bonus du proxy.
func (a *App) TMDBGetByImdbID(imdbID string) (*tmdb.Movie, error) {
	return a.tmdbClient().GetByImdbID(imdbID)
}

// TMDBGetProviders : liste les plateformes streaming par pays pour un titre.
func (a *App) TMDBGetProviders(tmdbID int, mediaType string) (map[string]tmdb.CountryProviders, error) {
	return a.tmdbClient().GetProviders(tmdbID, mediaType)
}

// --- Image proxy (contourne les restrictions CSP de Wails) ---

func (a *App) FetchImageBase64(url string) (string, error) {
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/jpeg"
	}
	return "data:" + ct + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// --- Parser ---

func (a *App) ParseFilename(filename string) *parser.FileInfo {
	return parser.ParseFilename(filename)
}

// --- Hydracker meta ---

func (a *App) GetMetaQualities() ([]api.Quality, error) {
	a.resetCancellation()
	return a.client.GetQualities()
}

func (a *App) GetMetaLangs() ([]api.Lang, error) {
	a.resetCancellation()
	return a.client.GetLangs()
}

func (a *App) GetMetaSubs() ([]api.Lang, error) {
	a.resetCancellation()
	return a.client.GetSubs()
}

// --- Hydracker search ---

func (a *App) HydrackerSearch(query string) ([]api.PartialTitle, error) {
	a.resetCancellation()
	result, err := a.client.Search(query, 10)
	if err != nil {
		return nil, err
	}
	return result.Titles, nil
}

func (a *App) HydrackerGetByTmdbID(tmdbID int) (*api.PartialTitle, error) {
	a.resetCancellation()
	return a.client.GetTitleByTmdbID(tmdbID)
}

func (a *App) HydrackerGetByID(id int) (*api.PartialTitle, error) {
	a.resetCancellation()
	title, err := a.client.GetTitle(id, 0, 0, false)
	if err != nil {
		return nil, err
	}
	partial := &api.PartialTitle{
		ID:          title.ID,
		Name:        title.Name,
		Type:        title.Type,
		Poster:      title.Poster,
		ReleaseDate: title.ReleaseDate,
		Score:       title.Score,
	}
	return partial, nil
}

func (a *App) OpenBrowser(url string) {
	wailsruntime.BrowserOpenURL(a.ctx, url)
}

// OpenHydrackerAdmin ouvre le panneau admin liens dans le navigateur (site configuré).
func (a *App) OpenHydrackerAdmin() {
	site := a.client.SiteURL()
	if site == "" {
		return
	}
	wailsruntime.BrowserOpenURL(a.ctx, site+"/admin/liens")
}

// --- Lecture fichier pour MediaInfo ---

func (a *App) GetFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (a *App) ReadFileChunk(path string, offset int64, size int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, 0); err != nil {
		return nil, err
	}
	buf := make([]byte, size)
	n, err := io.ReadFull(f, buf)
	if err == io.ErrUnexpectedEOF {
		err = nil
	}
	return buf[:n], err
}

// --- Sélection de fichiers ---

func (a *App) SelectMkvFile() (string, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Sélectionner un fichier MKV",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Fichiers vidéo", Pattern: "*.mkv;*.mp4"},
		},
	})
	return path, err
}

// SelectMkvFiles permet de choisir plusieurs fichiers MKV à la fois.
func (a *App) SelectMkvFiles() ([]string, error) {
	paths, err := wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Sélectionner un ou plusieurs fichiers MKV",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Fichiers vidéo", Pattern: "*.mkv;*.mp4"},
		},
	})
	return paths, err
}

func (a *App) SelectTorrentFile() (string, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Sélectionner un fichier .torrent",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Fichiers Torrent", Pattern: "*.torrent"},
		},
	})
	return path, err
}

func (a *App) SelectNzbFile() (string, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Sélectionner un fichier .nzb",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Fichiers NZB", Pattern: "*.nzb"},
		},
	})
	return path, err
}

// --- Upload Hydracker ---

func (a *App) PostTorrent(titleID, qualite int, langues, subs []string, torrentPath, nfo string, saison, episode int) (*api.UploadTorrentResult, error) {
	return a.client.UploadTorrent(titleID, qualite, langues, subs, torrentPath, nfo, saison, episode)
}

func (a *App) PostNzb(titleID, qualite int, langues, subs []string, nzbPath, nfo string, saison, episode int) (*api.UploadNzbResult, error) {
	return a.client.UploadNzb(titleID, qualite, langues, subs, nzbPath, nfo, saison, episode)
}

func (a *App) PostLien(titleID, qualite int, langues, subs []string, lien, nfo string, saison, episode int) (*api.UploadLienResult, error) {
	return a.client.UploadLien(titleID, qualite, langues, subs, lien, nfo, saison, episode)
}

// --- Reseed (MKV + .torrent → FTP + ruTorrent + re-check) ---

type ReseedPrepareResult struct {
	TorrentName   string             `json:"torrent_name"`
	FirstFileName string             `json:"first_file_name"`
	Size          int64              `json:"size"`
	InfoHash      string             `json:"info_hash"`
	Search        *mediasearch.SearchResult `json:"search"`
	HydrackerFiche *api.PartialTitle `json:"hydracker_fiche"`
}

func (a *App) ReseedPrepare(torrentPath string) (*ReseedPrepareResult, error) {
	mi, err := metainfo.LoadFromFile(torrentPath)
	if err != nil {
		return nil, fmt.Errorf("lecture .torrent : %w", err)
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return nil, fmt.Errorf("parse info : %w", err)
	}
	result := &ReseedPrepareResult{
		TorrentName: info.Name,
		InfoHash:    mi.HashInfoBytes().HexString(),
	}
	if len(info.Files) > 0 {
		result.FirstFileName = info.Files[0].DisplayPath(&info)
		result.Size = info.Files[0].Length
	} else {
		result.FirstFileName = info.Name
		result.Size = info.Length
	}
	// Recherche TMDB (prend le 1er résultat ; pour reseed on ne propose pas de choix)
	if results, err := mediasearch.Search(a.cfg.MediaSearchURL, info.Name); err == nil && len(results) > 0 {
		result.Search = &results[0]
		if results[0].TmdbID > 0 {
			if fiche, err := a.client.GetTitleByTmdbID(results[0].TmdbID); err == nil {
				result.HydrackerFiche = fiche
			}
		}
	}
	return result, nil
}

// MediaSearch expose la recherche multi-résultats pour la modal de choix côté UI.
func (a *App) MediaSearch(query string) ([]mediasearch.SearchResult, error) {
	return mediasearch.Search(a.cfg.MediaSearchURL, query)
}

// --- Admin ---

func (a *App) DeleteTorrent(id int) error { return a.client.DeleteTorrent(id) }
func (a *App) DeleteNzb(id int) error     { return a.client.DeleteNzb(id) }
func (a *App) DeleteLien(id int) error    { return a.client.DeleteLien(id) }

type FicheContent struct {
	Torrents *api.TorrentsResult `json:"torrents"`
	Nzbs     *api.NzbsResult     `json:"nzbs"`
	Liens    *api.LiensResult    `json:"liens"`
	Charged  float64             `json:"charged_total"`
}

func (a *App) GetMyUsername() (string, error) {
	u, err := a.client.GetMe()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

// GetUserProfile retourne le profil complet (stats incluses) pour un user.
// idOrUsername : "me", ID numérique ou pseudo — selon ce que l'API accepte.
func (a *App) GetUserProfile(idOrUsername string) (*api.User, error) {
	a.resetCancellation()
	return a.client.GetUser(idOrUsername)
}

func (a *App) FicheGetContent(titleID int) (*FicheContent, error) {
	a.resetCancellation()
	f := api.ContentFilter{}
	out := &FicheContent{}
	logErr := func(kind string, err error) {
		wailsruntime.EventsEmit(a.ctx, "check:log", "[fiche "+kind+" ERR] "+err.Error())
	}
	if t, err := a.client.GetTorrents(titleID, f); err != nil {
		logErr("torrents", err)
	} else {
		out.Torrents = t
		out.Charged += t.Charged
	}
	if n, err := a.client.GetNzbs(titleID, f); err != nil {
		logErr("nzbs", err)
	} else {
		out.Nzbs = n
		out.Charged += n.Charged
	}
	if l, err := a.client.GetLiens(titleID, f); err != nil {
		logErr("liens", err)
	} else {
		out.Liens = l
		out.Charged += l.Charged
	}
	if api.LastRawTorrents != "" {
		raw := api.LastRawTorrents
		if len(raw) > 400 {
			raw = raw[:400] + "…"
		}
		wailsruntime.EventsEmit(a.ctx, "check:log", "[torrents raw] "+raw)
	}
	return out, nil
}

// --- Team gating : login pseudo + bcrypt ---
//
// team.json (sur GitHub raw, facile à éditer sans rebuild) contient pour
// chaque user : pseudo, role, title, password_hash (bcrypt).
// L'app affiche un écran de login au démarrage, vérifie le hash en local,
// et stocke la session UNIQUEMENT en mémoire (rien dans config.json).
// Pas de dépendance à Hydracker pour l'identité — impossible d'usurper un
// autre user même en bidouillant le token.

const teamListURL = "https://raw.githubusercontent.com/Gandalfleblanc/Go-Post-Tools/main/team.json"

type TeamUser struct {
	Pseudo       string `json:"pseudo"`
	Role         string `json:"role"`
	Title        string `json:"title,omitempty"`
	PasswordHash string `json:"password_hash"`
}

// RoleDef : définition d'un rôle (badge, couleur, tabs visibles).
type RoleDef struct {
	Badge string   `json:"badge"`           // emoji affiché (🥇, 🥈, …)
	Color string   `json:"color"`           // couleur CSS (#fbbf24, …)
	Title string   `json:"title,omitempty"` // libellé défaut si pas de title user
	Tabs  []string `json:"tabs"`            // liste des IDs de tabs visibles
}

type teamList struct {
	Roles map[string]RoleDef `json:"roles,omitempty"`
	Users []TeamUser         `json:"users"`
}

// defaultRoles : valeurs par défaut si team.json n'a pas de section "roles".
// Permet la rétrocompat avec l'ancien team.json (v5.0.x/v5.1.0).
func defaultRoles() map[string]RoleDef {
	return map[string]RoleDef{
		"admin": {Badge: "🥇", Color: "#fbbf24", Title: "Admin",
			Tabs: []string{"hydracker", "fiches", "check", "reseed", "myuploads", "history", "logs", "manager", "settings"}},
		"modo": {Badge: "🥈", Color: "#cbd5e1", Title: "Modo",
			Tabs: []string{"hydracker", "fiches", "check", "reseed", "myuploads", "history", "logs", "settings"}},
		"team": {Badge: "🥉", Color: "#cd7f32", Title: "Team",
			Tabs: []string{"hydracker", "fiches", "check", "reseed", "myuploads", "history", "settings"}},
		"user": {Badge: "🔵", Color: "#60a5fa", Title: "User",
			Tabs: []string{"hydracker", "fiches", "myuploads", "history", "settings"}},
	}
}

// AuthResult : résultat de LoginUser / GetCurrentUser.
type AuthResult struct {
	Username string   `json:"username"`
	Role     string   `json:"role"`
	Title    string   `json:"title"`
	Avatar   string   `json:"avatar,omitempty"`
	Badge    string   `json:"badge,omitempty"` // emoji du rôle (résolu depuis roles)
	Color    string   `json:"color,omitempty"` // couleur CSS du rôle
	Tabs     []string `json:"tabs,omitempty"`  // tabs visibles pour ce rôle
}

// TeamConfig : renvoyé à l'UI Manager (admin only).
type TeamConfig struct {
	Roles map[string]RoleDef `json:"roles"`
	Users []TeamUser         `json:"users"` // sans password_hash
}

// fetchTeam récupère team.json depuis GitHub raw (avec cache-buster).
func fetchTeam() (*teamList, error) {
	url := fmt.Sprintf("%s?_=%d", teamListURL, time.Now().Unix())
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "GoPostTools/5.x")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch team.json : %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fetch team.json HTTP %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var tl teamList
	if err := json.Unmarshal(body, &tl); err != nil {
		return nil, fmt.Errorf("parse team.json : %w", err)
	}
	return &tl, nil
}

// resolveRoles : renvoie le map de rôles (depuis team.json, sinon defaults).
func resolveRoles(tl *teamList) map[string]RoleDef {
	if tl.Roles != nil && len(tl.Roles) > 0 {
		return tl.Roles
	}
	return defaultRoles()
}

// LoginUser : pseudo + mot de passe → vérifie contre team.json (bcrypt).
// En cas de succès, stocke l'user en session mémoire et le retourne.
func (a *App) LoginUser(pseudo, password string) (*AuthResult, error) {
	pseudo = strings.TrimSpace(pseudo)
	if pseudo == "" || password == "" {
		return nil, fmt.Errorf("pseudo et mot de passe requis")
	}
	tl, err := fetchTeam()
	if err != nil {
		return nil, err
	}
	roles := resolveRoles(tl)
	for _, u := range tl.Users {
		if !strings.EqualFold(u.Pseudo, pseudo) {
			continue
		}
		if u.PasswordHash == "" {
			return nil, fmt.Errorf("utilisateur %q sans password_hash — contacte l'admin", u.Pseudo)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
			return nil, fmt.Errorf("mot de passe incorrect")
		}
		role := u.Role
		def, ok := roles[role]
		if !ok {
			// rôle non défini → fallback sur "user" si existant, sinon minimum
			if userDef, uok := roles["user"]; uok {
				def = userDef
				role = "user"
			} else {
				def = RoleDef{Badge: "🔵", Color: "#60a5fa", Tabs: []string{"hydracker", "settings"}}
			}
		}
		title := u.Title
		if title == "" {
			title = def.Title
		}
		res := &AuthResult{
			Username: u.Pseudo,
			Role:     role,
			Title:    title,
			Badge:    def.Badge,
			Color:    def.Color,
			Tabs:     def.Tabs,
		}
		a.sessionMu.Lock()
		a.currentUser = res
		a.sessionMu.Unlock()
		return res, nil
	}
	return nil, fmt.Errorf("pseudo inconnu")
}

// GetTeamConfig : renvoie la config complète team.json (roles + users sans hash).
// Accessible uniquement à un admin connecté — pour l'onglet Manager.
func (a *App) GetTeamConfig() (*TeamConfig, error) {
	a.sessionMu.Lock()
	cur := a.currentUser
	a.sessionMu.Unlock()
	if cur == nil || cur.Role != "admin" {
		return nil, fmt.Errorf("accès réservé aux admins")
	}
	tl, err := fetchTeam()
	if err != nil {
		return nil, err
	}
	out := &TeamConfig{
		Roles: resolveRoles(tl),
		Users: make([]TeamUser, 0, len(tl.Users)),
	}
	for _, u := range tl.Users {
		// on ne renvoie pas le hash pour éviter de l'exposer inutilement
		out.Users = append(out.Users, TeamUser{
			Pseudo: u.Pseudo,
			Role:   u.Role,
			Title:  u.Title,
		})
	}
	return out, nil
}

// BuildTeamJSON : génère le JSON complet team.json à partir d'une config éditée dans le Manager.
// Les entries users peuvent contenir password_hash vide → on réutilise l'ancien hash si on
// reconnaît le pseudo (pas de reset accidentel de mdp quand on bouge juste un rôle).
// Si un nouveau password est fourni pour un user existant, il remplace l'ancien.
func (a *App) BuildTeamJSON(roles map[string]RoleDef, users []TeamUser, newPasswords map[string]string) (string, error) {
	a.sessionMu.Lock()
	cur := a.currentUser
	a.sessionMu.Unlock()
	if cur == nil || cur.Role != "admin" {
		return "", fmt.Errorf("accès réservé aux admins")
	}
	// Récupère l'ancien team.json pour réutiliser les hashs existants
	prev, err := fetchTeam()
	if err != nil {
		return "", fmt.Errorf("fetch team.json actuel : %w", err)
	}
	prevHashes := map[string]string{}
	for _, u := range prev.Users {
		prevHashes[strings.ToLower(u.Pseudo)] = u.PasswordHash
	}

	out := teamList{Roles: roles, Users: make([]TeamUser, 0, len(users))}
	for _, u := range users {
		key := strings.ToLower(u.Pseudo)
		hash := u.PasswordHash
		if hash == "" {
			// pas de hash fourni → on tente de réutiliser l'ancien
			hash = prevHashes[key]
		}
		if np, ok := newPasswords[u.Pseudo]; ok && np != "" {
			// nouveau mdp fourni → regénère
			b, err := bcrypt.GenerateFromPassword([]byte(np), 12)
			if err != nil {
				return "", fmt.Errorf("hash %s : %w", u.Pseudo, err)
			}
			hash = string(b)
		}
		if hash == "" {
			return "", fmt.Errorf("utilisateur %q sans mot de passe (ni nouveau ni ancien)", u.Pseudo)
		}
		out.Users = append(out.Users, TeamUser{
			Pseudo:       u.Pseudo,
			Role:         u.Role,
			Title:        u.Title,
			PasswordHash: hash,
		})
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Logout : clear la session mémoire.
func (a *App) Logout() {
	a.sessionMu.Lock()
	a.currentUser = nil
	a.sessionMu.Unlock()
}

// GetCurrentUser : renvoie l'user courant (ou nil si pas connecté).
func (a *App) GetCurrentUser() *AuthResult {
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()
	return a.currentUser
}

// HashPassword : génère un hash bcrypt pour un mot de passe.
// Utilisé par l'UI admin pour créer des entrées team.json.
func (a *App) HashPassword(password string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("mot de passe vide")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// FetchHydrackerAvatar : récupère l'avatar du user courant via son token Hydracker.
// Called post-login — optionnel, fallback silencieux si token absent/invalide.
// Renvoie l'URL absolue de l'avatar (ou "" si pas dispo).
func (a *App) FetchHydrackerAvatar() string {
	if a.cfg == nil || strings.TrimSpace(a.cfg.HydrackerToken) == "" {
		return ""
	}
	me, err := a.client.GetMe()
	if err != nil || me == nil {
		return ""
	}
	av := me.Image
	if av == "" {
		av = me.Avatar
	}
	if av == "" {
		return ""
	}
	if !strings.HasPrefix(av, "http") {
		// Chemin relatif → préfixe l'URL de base Hydracker
		base := strings.TrimRight(a.cfg.HydrackerBaseURL, "/")
		// Enlève /api/v1 si présent
		base = strings.TrimSuffix(base, "/api/v1")
		av = base + "/" + strings.TrimLeft(av, "/")
	}
	// Mets à jour la session mémoire si connecté
	a.sessionMu.Lock()
	if a.currentUser != nil {
		a.currentUser.Avatar = av
	}
	a.sessionMu.Unlock()
	return av
}

// ChangeMyPassword : génère un nouveau hash pour l'user connecté + renvoie le team.json
// complet à coller sur GitHub (tous les autres hashs préservés).
func (a *App) ChangeMyPassword(newPassword string) (string, error) {
	a.sessionMu.Lock()
	cur := a.currentUser
	a.sessionMu.Unlock()
	if cur == nil {
		return "", fmt.Errorf("non connecté")
	}
	if strings.TrimSpace(newPassword) == "" {
		return "", fmt.Errorf("nouveau mot de passe vide")
	}
	prev, err := fetchTeam()
	if err != nil {
		return "", fmt.Errorf("fetch team.json actuel : %w", err)
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil {
		return "", err
	}
	out := teamList{Roles: resolveRoles(prev), Users: make([]TeamUser, 0, len(prev.Users))}
	found := false
	for _, u := range prev.Users {
		if strings.EqualFold(u.Pseudo, cur.Username) {
			u.PasswordHash = string(newHash)
			found = true
		}
		out.Users = append(out.Users, u)
	}
	if !found {
		return "", fmt.Errorf("user %q absent de team.json ?!", cur.Username)
	}
	// Si team.json n'avait pas de section roles, on la matérialise avec les defaults
	// pour éviter que le user perde ses tabs au prochain fetch.
	if prev.Roles == nil || len(prev.Roles) == 0 {
		out.Roles = defaultRoles()
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// NzbFileEntry : un fichier listé dans un NZB.
type NzbFileEntry struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size,omitempty"`
}

// GetNzbFilenames : fetch le XML NZB depuis Hydracker et extrait les noms de fichiers.
// nzbID = id du NZB sur Hydracker.
func (a *App) GetNzbFilenames(nzbID int) ([]NzbFileEntry, error) {
	if a.client == nil {
		return nil, fmt.Errorf("client non initialisé")
	}
	xmlData, err := a.downloadHydrackerNzb(nzbID)
	if err != nil {
		return nil, fmt.Errorf("download NZB : %w", err)
	}
	return parseNzbFiles(xmlData), nil
}

// downloadHydrackerNzb : télécharge le XML d'un NZB via GET /api/v1/nzbs/{id}/download.
func (a *App) downloadHydrackerNzb(nzbID int) ([]byte, error) {
	apiURL := fmt.Sprintf("%s/nzbs/%d/download", a.client.BaseURL(), nzbID)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	if a.cfg.HydrackerToken != "" {
		req.Header.Set("Authorization", "Bearer "+a.cfg.HydrackerToken)
	}
	req.Header.Set("Accept", "application/x-nzb, text/xml, */*")
	req.Header.Set("User-Agent", "GoPostTools/5.x")
	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return data, nil
}

// parseNzbFiles : parse un XML NZB et extrait les filenames depuis les attributs subject.
// Les filenames dans les NZB sont dans <file subject="[1/42] - \"nom.mkv\" yEnc (1/40)">.
func parseNzbFiles(xmlData []byte) []NzbFileEntry {
	out := []NzbFileEntry{}
	seen := map[string]bool{}
	s := string(xmlData)
	idx := 0
	for {
		i := strings.Index(s[idx:], "<file ")
		if i < 0 {
			break
		}
		i += idx
		close := strings.Index(s[i:], ">")
		if close < 0 {
			break
		}
		tag := s[i : i+close]
		idx = i + close + 1
		// Extract subject="..."
		subIdx := strings.Index(tag, "subject=\"")
		if subIdx < 0 {
			continue
		}
		subIdx += len("subject=\"")
		end := strings.Index(tag[subIdx:], "\"")
		if end < 0 {
			continue
		}
		subject := tag[subIdx : subIdx+end]
		// Unescape XML entities basiques
		subject = strings.ReplaceAll(subject, "&quot;", "\"")
		subject = strings.ReplaceAll(subject, "&apos;", "'")
		subject = strings.ReplaceAll(subject, "&amp;", "&")
		// Extract le filename entre guillemets dans le subject (si présent)
		fn := extractFilenameFromSubject(subject)
		if fn == "" {
			continue
		}
		if seen[fn] {
			continue
		}
		seen[fn] = true
		out = append(out, NzbFileEntry{Filename: fn})
	}
	return out
}

// extractFilenameFromSubject : heuristique pour récupérer "nom.ext" dans un subject yEnc.
// Formats fréquents :
//
//	[1/5] - "archive.par2" yEnc (1/1)
//	"archive.par2" yEnc (1/1)
func extractFilenameFromSubject(subj string) string {
	// Cherche d'abord entre guillemets
	if i := strings.Index(subj, "\""); i >= 0 {
		if j := strings.Index(subj[i+1:], "\""); j > 0 {
			return subj[i+1 : i+1+j]
		}
	}
	// Fallback : récupère le mot qui ressemble à un filename (contient un point)
	for _, tok := range strings.Fields(subj) {
		if strings.Contains(tok, ".") && !strings.Contains(tok, "/") {
			return strings.Trim(tok, "[]()-")
		}
	}
	return ""
}

// FicheGetNfo récupère le NFO HTML d'un torrent/lien/nzb.
// kind = "torrents" | "liens" | "nzbs"
func (a *App) FicheGetNfo(kind string, id int) (string, error) {
	a.resetCancellation()
	return a.client.GetNfo(kind, id)
}

// UploaderRow agrège l'activité d'un uploader sur la fenêtre de scan.
type UploaderRow struct {
	Author       string `json:"author"`
	Torrents     int    `json:"torrents"`
	Nzbs         int    `json:"nzbs"`
	Liens        int    `json:"liens"`
	Total        int    `json:"total"`
	TotalSize    int64  `json:"total_size"`
	LastUploadAt string `json:"last_upload_at"`
}

type UploaderScanResult struct {
	Uploaders     []UploaderRow `json:"uploaders"`
	ScannedTitles int           `json:"scanned_titles"`
	ScannedItems  struct {
		Torrents int `json:"torrents"`
		Nzbs     int `json:"nzbs"`
		Liens    int `json:"liens"`
	} `json:"scanned_items"`
	OldestScanned string `json:"oldest_scanned"` // date de la plus ancienne fiche scannée
	NewestScanned string `json:"newest_scanned"`
	DurationSec   int    `json:"duration_sec"`
}

// GetTopUploaders construit un classement site-wide en :
//   1. Listant les N fiches les plus populaires (`/titles?order=popularity:desc`)
//   2. Pour chaque fiche, fetchant ses torrents/nzbs/liens via les endpoints
//      documentés `/titles/{id}/content/{type}` (qui exposent l'auteur)
//   3. Agrégeant par uploader, retournant top topN par type
// Coût : 1 + 3*scanTitles requêtes, sérialisées par le rate limit (1 req/s).
// Avec scanTitles=30 → ~91 req → ~95 secondes.
func (a *App) GetUploaderStats(daysSince int) (*UploaderScanResult, error) {
	if daysSince <= 0 {
		daysSince = 30
	}
	startTime := time.Now()
	cutoff := time.Now().UTC().AddDate(0, 0, -daysSince).Format(time.RFC3339)
	result := &UploaderScanResult{}

	uploaders := map[string]*UploaderRow{}
	getOrCreate := func(name string) *UploaderRow {
		if u, ok := uploaders[name]; ok {
			return u
		}
		u := &UploaderRow{Author: name}
		uploaders[name] = u
		return u
	}
	updateLast := func(u *UploaderRow, ts string) {
		if ts > u.LastUploadAt {
			u.LastUploadAt = ts
		}
	}

	// 1) TORRENTS : endpoint global /torrents (perPage=100, sorted desc).
	//    On paginate jusqu'à hit créé avant le cutoff.
	page := 1
	for {
		r, err := a.client.GetGlobalTorrents(page, 100)
		if err != nil || len(r.Pagination.Data) == 0 {
			break
		}
		stop := false
		for _, item := range r.Pagination.Data {
			if item.CreatedAt < cutoff {
				stop = true
				continue
			}
			if item.Author == "" {
				continue
			}
			u := getOrCreate(item.Author)
			u.Torrents++
			u.Total++
			u.TotalSize += item.Size
			updateLast(u, item.CreatedAt)
			result.ScannedItems.Torrents++
		}
		if stop {
			break
		}
		page++
		if page > 200 {
			break
		} // safety
	}

	// 2) FICHES actives dans la fenêtre : on liste /titles ordonné par
	//    last_content_added_at desc, et pour chacune on fetch ses NZB et DDL.
	titlePage := 1
	for {
		titles, err := a.client.GetTitles(api.TitleFilter{Order: "last_content_added_at:desc", PerPage: 100, Page: titlePage})
		if err != nil || len(titles.Data) == 0 {
			break
		}
		stop := false
		for _, t := range titles.Data {
			if t.LastContentAddedAt < cutoff {
				stop = true
				break
			}
			result.ScannedTitles++
			if result.NewestScanned == "" {
				result.NewestScanned = t.LastContentAddedAt
			}
			result.OldestScanned = t.LastContentAddedAt

			if r, err := a.client.GetNzbs(t.ID, api.ContentFilter{}); err == nil {
				for _, item := range r.Nzbs {
					if item.CreatedAt < cutoff {
						continue
					}
					name := item.Author
					if name == "" {
						name = item.IDUser
					}
					if name == "" {
						continue
					}
					u := getOrCreate(name)
					u.Nzbs++
					u.Total++
					u.TotalSize += item.Size
					updateLast(u, item.CreatedAt)
					result.ScannedItems.Nzbs++
				}
			}
			if r, err := a.client.GetLiens(t.ID, api.ContentFilter{}); err == nil {
				for _, item := range r.Liens {
					if item.CreatedAt < cutoff {
						continue
					}
					if item.IDUser == "" {
						continue
					}
					u := getOrCreate(item.IDUser)
					u.Liens++
					u.Total++
					u.TotalSize += item.Size
					updateLast(u, item.CreatedAt)
					result.ScannedItems.Liens++
				}
			}
		}
		if stop {
			break
		}
		titlePage++
		if titlePage > 50 {
			break
		} // safety
	}

	rows := make([]UploaderRow, 0, len(uploaders))
	for _, u := range uploaders {
		rows = append(rows, *u)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Total > rows[j].Total })
	result.Uploaders = rows
	result.DurationSec = int(time.Since(startTime).Seconds())
	return result, nil
}

// GetDDLFilename résout le nom de fichier réel derrière une URL DDL (1fichier
// pour l'instant). Utilisé dans la section Fiches pour afficher le vrai
// filename à côté de chaque lien partagé.
func (a *App) GetDDLFilename(shareURL string) (string, error) {
	if shareURL == "" {
		return "", fmt.Errorf("URL vide")
	}
	if strings.Contains(shareURL, "1fichier.com") {
		info, err := downloader.OneFichierGetInfo(context.Background(), a.cfg.OneFichierApiKey, shareURL)
		if err != nil {
			return "", err
		}
		return info.Filename, nil
	}
	return "", fmt.Errorf("host non supporté pour résolution de filename")
}

func (a *App) ReseedExecute(torrentPath, mkvPath string) error {
	if a.cfg.FTPHost == "" {
		return fmt.Errorf("FTP non configuré")
	}
	if a.cfg.SeedboxURL == "" {
		return fmt.Errorf("seedbox non configurée")
	}
	emit := func(stage, msg string) {
		wailsruntime.EventsEmit(a.ctx, "reseed:status", map[string]interface{}{"stage": stage, "msg": msg})
	}
	// 1. Upload MKV sur FTP
	emit("ftp", "Upload MKV sur FTP…")
	remoteName, err := ftpup.Upload(a.workContext(), a.cfg.FTPHost, a.cfg.FTPPort, a.cfg.FTPUser, a.cfg.FTPPassword, a.cfg.FTPPath, mkvPath, func(p ftpup.Progress) {
		wailsruntime.EventsEmit(a.ctx, "reseed:progress", p)
	})
	if err != nil {
		emit("error", "ftp: "+err.Error())
		return err
	}
	emit("ftp_done", "FTP OK : "+remoteName)

	// 2. Upload .torrent sur ruTorrent (addtorrent.php)
	emit("seedbox", "Ajout sur ruTorrent…")
	if _, err := a.pushTorrent(a.workContext(), torrentPath, "admin", nil); err != nil {
		emit("error", "seedbox: "+err.Error())
		return err
	}

	// 3. Force re-check (le torrent vient d'être ajouté, il va check le MKV uploadé)
	mi, err := metainfo.LoadFromFile(torrentPath)
	if err == nil {
		hash := mi.HashInfoBytes().HexString()
		emit("recheck", "Re-check…")
		time.Sleep(2 * time.Second)
		_ = a.recheckTorrent(hash, "admin")
	}
	emit("done", "Terminé")
	return nil
}

// --- Seedbox abstraction : choix explicite ou auto qBit vs ruTorrent ---
//
// seedboxType :
//   - "admin"  : ruTorrent team-shared Gandalf (SeedboxURL)
//   - "prive"  : ruTorrent perso de l'user (PrivateSeedboxURL)
//   - "modo"   : qBittorrent team-shared (QBitURL)
//   - ""       : auto (qBit prioritaire si configuré, sinon ruTorrent admin)
//
// pushTorrent envoie un .torrent au seedbox cible. onProgress peut être nil.
func (a *App) pushTorrent(ctx context.Context, torrentPath, seedboxType string, onProgress func(seedbox.Progress)) (string, error) {
	if seedboxType == "prive" {
		// qBit privé prioritaire si configuré, sinon ruTorrent privé
		if a.cfg.PrivateQBitURL != "" {
			return qbittorrent.Upload(ctx, a.cfg.PrivateQBitURL, a.cfg.PrivateQBitUser, a.cfg.PrivateQBitPassword, a.cfg.PrivateSeedboxLabel, torrentPath, func(p qbittorrent.Progress) {
				if onProgress != nil {
					onProgress(seedbox.Progress{Percent: p.Percent, SpeedMB: p.SpeedMB})
				}
			})
		}
		if a.cfg.PrivateSeedboxURL != "" {
			return seedbox.Upload(ctx, a.cfg.PrivateSeedboxURL, a.cfg.PrivateSeedboxUser, a.cfg.PrivateSeedboxPassword, a.cfg.PrivateSeedboxLabel, torrentPath, onProgress)
		}
		return "", fmt.Errorf("seedbox Privée non configurée (Réglages : ruTorrent ou qBittorrent)")
	}
	useQBit := seedboxType == "modo" || (seedboxType == "" && a.cfg.QBitURL != "")
	useRuTorrent := seedboxType == "admin" || (seedboxType == "" && a.cfg.QBitURL == "" && a.cfg.SeedboxURL != "")
	if useQBit {
		if a.cfg.QBitURL == "" {
			return "", fmt.Errorf("seedbox MODO (qBittorrent) non configurée")
		}
		return qbittorrent.Upload(ctx, a.cfg.QBitURL, a.cfg.QBitUser, a.cfg.QBitPassword, a.cfg.SeedboxLabel, torrentPath, func(p qbittorrent.Progress) {
			if onProgress != nil {
				onProgress(seedbox.Progress{Percent: p.Percent, SpeedMB: p.SpeedMB})
			}
		})
	}
	if useRuTorrent {
		if a.cfg.SeedboxURL == "" {
			return "", fmt.Errorf("seedbox ADMIN (ruTorrent) non configurée")
		}
		return seedbox.Upload(ctx, a.cfg.SeedboxURL, a.cfg.SeedboxUser, a.cfg.SeedboxPassword, a.cfg.SeedboxLabel, torrentPath, onProgress)
	}
	return "", fmt.Errorf("aucune seedbox configurée (Réglages : ruTorrent ou qBittorrent)")
}

// recheckTorrent force un recheck côté seedbox cible.
func (a *App) recheckTorrent(hash, seedboxType string) error {
	if seedboxType == "prive" {
		if a.cfg.PrivateQBitURL != "" {
			return qbittorrent.Recheck(a.cfg.PrivateQBitURL, a.cfg.PrivateQBitUser, a.cfg.PrivateQBitPassword, hash)
		}
		if a.cfg.PrivateSeedboxURL != "" {
			return rutorrent.Recheck(a.cfg.PrivateSeedboxURL, a.cfg.PrivateSeedboxUser, a.cfg.PrivateSeedboxPassword, hash)
		}
		return fmt.Errorf("seedbox Privée non configurée")
	}
	useQBit := seedboxType == "modo" || (seedboxType == "" && a.cfg.QBitURL != "")
	useRuTorrent := seedboxType == "admin" || (seedboxType == "" && a.cfg.QBitURL == "" && a.cfg.SeedboxURL != "")
	if useQBit && a.cfg.QBitURL != "" {
		return qbittorrent.Recheck(a.cfg.QBitURL, a.cfg.QBitUser, a.cfg.QBitPassword, hash)
	}
	if useRuTorrent && a.cfg.SeedboxURL != "" {
		return rutorrent.Recheck(a.cfg.SeedboxURL, a.cfg.SeedboxUser, a.cfg.SeedboxPassword, hash)
	}
	return fmt.Errorf("seedbox %q non configurée", seedboxType)
}

// seedboxConfigured indique si au moins une seedbox (ruTorrent ou qBit) est configurée.
func (a *App) seedboxConfigured() bool {
	return a.cfg.QBitURL != "" || a.cfg.SeedboxURL != ""
}

// findExistingTorrent cherche un torrent existant sur Hydracker qui matche
// soit l'info_hash (via /content/torrents) soit le torrent_name (via /admin/torrents).
// Utilisé pour dedup avant UploadTorrent — évite un 422 "Torrent existe déjà"
// si un retry précédent a laissé le 1er post en place.
func (a *App) findExistingTorrent(titleID int, infoHash, torrentName string) (int, error) {
	if titleID <= 0 {
		return 0, nil
	}
	targetHash := strings.ToLower(infoHash)
	targetName := strings.TrimSpace(torrentName)

	// Essai 1 : endpoint public /content/torrents (a info_hash si opt-in API)
	if targetHash != "" {
		if res, err := a.client.GetTorrents(titleID, api.ContentFilter{}); err == nil {
			for _, t := range res.Torrents {
				if strings.ToLower(t.InfoHash) == targetHash || strings.ToLower(t.Hash) == targetHash {
					return t.ID, nil
				}
			}
		}
	}

	// Essai 2 : /admin/torrents (a torrent_name, pas forcément info_hash).
	// On match par title_id + torrent_name (qui reste stable entre retries).
	if targetName != "" {
		for page := 1; page <= 3; page++ {
			res, err := a.client.ListAdminTorrents("", page)
			if err != nil {
				break
			}
			for _, t := range res.Pagination.Data {
				if t.TitleID != titleID {
					continue
				}
				if strings.EqualFold(t.Name, targetName) {
					return t.ID, nil
				}
			}
			if res.Pagination.CurrentPage >= res.Pagination.LastPage {
				break
			}
		}
	}
	return 0, nil
}

func (a *App) SelectAnyTorrentFile() (string, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Sélectionner un fichier .torrent",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Fichiers Torrent", Pattern: "*.torrent"},
		},
	})
	return path, err
}

// --- Check Torrent (liste ruTorrent + matching LiHDL + re-seed auto) ---

type CheckTorrentEntry struct {
	Hash       string `json:"hash"`
	Name       string `json:"name"`
	FileName   string `json:"file_name"`
	HasError   bool   `json:"has_error"`
	Message    string `json:"message"`
	IsActive   int    `json:"is_active"`
	State      int    `json:"state"`
	Size       int64  `json:"size"`
	Done       int64  `json:"done"`
	LihdlURL   string `json:"lihdl_url"`
	LihdlName  string `json:"lihdl_name"`
}

func (a *App) ListCheckTorrents(refreshIndex bool) ([]CheckTorrentEntry, error) {
	if a.cfg.SeedboxURL == "" {
		return nil, fmt.Errorf("seedbox non configurée")
	}
	list, err := rutorrent.List(a.cfg.SeedboxURL, a.cfg.SeedboxUser, a.cfg.SeedboxPassword)
	if err != nil {
		return nil, err
	}
	files, ferr := lihdl.FetchIndex(refreshIndex, a.cfg.LihdlBaseURL, a.cfg.LihdlUser, a.cfg.LihdlPassword)
	if ferr != nil {
		// on continue, juste pas de match possible
		wailsruntime.EventsEmit(a.ctx, "check:log", "⚠ LiHDL index : "+ferr.Error())
		files = nil
	}
	out := make([]CheckTorrentEntry, 0, len(list))
	for _, t := range list {
		entry := CheckTorrentEntry{
			Hash: t.Hash, Name: t.Name, FileName: t.FileName,
			HasError: t.HasError, Message: t.Message,
			IsActive: t.IsActive, State: t.State,
			Size: t.Size, Done: t.Done,
		}
		if files != nil {
			if m := lihdl.Match(t.Name, files); m != nil {
				entry.LihdlURL = m.URL
				entry.LihdlName = m.Name
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

// ReseedFromLihdl : stream LiHDL → upload FTP → force re-check ruTorrent.
func (a *App) ReseedFromLihdl(hash, lihdlURL, remoteName string) error {
	if a.cfg.FTPHost == "" {
		return fmt.Errorf("FTP non configuré")
	}
	if a.cfg.SeedboxURL == "" {
		return fmt.Errorf("seedbox non configurée")
	}
	if lihdlURL == "" {
		return fmt.Errorf("URL LiHDL manquante")
	}
	if remoteName == "" {
		return fmt.Errorf("nom distant manquant")
	}

	emit := func(stage, msg string) {
		wailsruntime.EventsEmit(a.ctx, "check:status", map[string]interface{}{"hash": hash, "stage": stage, "msg": msg})
	}
	emit("download", fmt.Sprintf("Download LiHDL : %s\n  URL: %s", remoteName, lihdlURL))

	req, _ := http.NewRequest("GET", lihdlURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15")
	req.Header.Set("Accept", "*/*")
	if a.cfg.LihdlUser != "" {
		req.SetBasicAuth(a.cfg.LihdlUser, a.cfg.LihdlPassword)
	}
	c := &http.Client{Timeout: 0}
	resp, err := c.Do(req)
	if err != nil {
		emit("error", "download: "+err.Error())
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("LiHDL HTTP %d", resp.StatusCode)
	}
	total := resp.ContentLength
	emit("download", fmt.Sprintf("Download LiHDL : %s (%.1f MB attendus)", remoteName, float64(total)/1024/1024))
	// Sanity check : si Content-Type est text/html, c'est probablement une page d'erreur
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "text/") {
		return fmt.Errorf("réponse serveur invalide (Content-Type %s) — URL/auth ?", ct)
	}

	// remoteName peut contenir un chemin absolu (ruTorrent get_base_path) → on prend juste le nom
	remoteName = filepath.Base(remoteName)
	emit("ftp", fmt.Sprintf("Upload FTP : %s", remoteName))
	if err := ftpup.UploadFromReader(a.workContext(), a.cfg.FTPHost, a.cfg.FTPPort, a.cfg.FTPUser, a.cfg.FTPPassword, a.cfg.FTPPath, remoteName, resp.Body, total, func(p ftpup.Progress) {
		wailsruntime.EventsEmit(a.ctx, "check:progress", map[string]interface{}{"hash": hash, "percent": p.Percent, "speed": p.SpeedMB})
	}); err != nil {
		emit("error", "ftp: "+err.Error())
		return err
	}

	emit("recheck", "Re-check seedbox…")
	if err := a.recheckTorrent(hash, "admin"); err != nil {
		emit("error", "recheck: "+err.Error())
		return err
	}
	emit("done", "Terminé")
	return nil
}

// --- Workflow Torrent complet (FTP → create → Hydracker → download → Seedbox) ---

type TorrentWorkflowResult struct {
	TorrentPath     string `json:"torrent_path"`
	HydrackerID     int    `json:"hydracker_id"`
	HydrackerTorPath string `json:"hydracker_torrent_path"` // chemin local du .torrent téléchargé depuis Hydracker
	SeedboxPath     string `json:"seedbox_path"`
}

// DownloadToDownloads télécharge un fichier via signed URL vers ~/Downloads avec un nom donné.
// Utilisé pour récupérer automatiquement les .torrent/NZB depuis Hydracker (download_url signée 5 min).
func (a *App) DownloadToDownloads(url, suggestedName string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("URL manquante")
	}
	home, _ := os.UserHomeDir()
	downloads := filepath.Join(home, "Downloads")
	_ = os.MkdirAll(downloads, 0755)
	// Nettoie le nom pour système de fichiers
	name := strings.TrimSpace(suggestedName)
	if name == "" {
		name = fmt.Sprintf("hydracker-%d.bin", time.Now().Unix())
	}
	for _, r := range []string{"/", "\\", ":", "?", "*", "|", "\""} {
		name = strings.ReplaceAll(name, r, "_")
	}
	outPath := filepath.Join(downloads, name)

	req, _ := http.NewRequestWithContext(a.workContext(), "GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	if a.cfg.HydrackerToken != "" {
		req.Header.Set("Authorization", "Bearer "+a.cfg.HydrackerToken)
	}
	c := &http.Client{Timeout: 5 * time.Minute}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}

	out, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", err
	}

	// Ouvre le Finder/Explorer sur le fichier téléchargé
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", "-R", outPath).Run()
	case "linux":
		_ = exec.Command("xdg-open", downloads).Run()
	case "windows":
		_ = exec.Command("explorer", "/select,", outPath).Run()
	}
	return outPath, nil
}

// FindHydrackerSourcesResult regroupe tous les contenus alternatifs disponibles sur Hydracker
// pour un titre/épisode donné — utile pour retrouver un .torrent ou un lien DDL quand un
// re-seed est nécessaire.
type FindHydrackerSourcesResult struct {
	Liens    []api.Lien        `json:"liens"`
	Nzbs     []api.Nzb         `json:"nzbs"`
	Torrents []api.TorrentItem `json:"torrents"`
}

// FindHydrackerSources interroge les 3 endpoints content (liens/nzbs/torrents) pour un titre
// (et éventuellement saison/épisode) et retourne toutes les sources disponibles.
func (a *App) FindHydrackerSources(titleID, saison, episode int) (*FindHydrackerSourcesResult, error) {
	a.resetCancellation()
	if titleID <= 0 {
		return nil, fmt.Errorf("title_id manquant")
	}
	logEv := func(msg string) {
		wailsruntime.EventsEmit(a.ctx, "check:log", "[FindSources] "+msg)
	}
	filter := api.ContentFilter{Season: saison, Episode: episode}
	logEv(fmt.Sprintf("title=%d saison=%d episode=%d", titleID, saison, episode))
	res := &FindHydrackerSourcesResult{}

	if liens, err := a.client.GetLiens(titleID, filter); err != nil {
		logEv("liens err: " + err.Error())
	} else {
		res.Liens = liens.Liens
		logEv(fmt.Sprintf("liens: %d (charged=%.3f€, already_paid=%d, count=%d)",
			len(liens.Liens), liens.Charged, liens.AlreadyPaid, liens.Count))
	}
	if nzbs, err := a.client.GetNzbs(titleID, filter); err != nil {
		logEv("nzbs err: " + err.Error())
	} else {
		res.Nzbs = nzbs.Nzbs
		logEv(fmt.Sprintf("nzbs: %d (charged=%.3f€)", len(nzbs.Nzbs), nzbs.Charged))
	}
	if torrents, err := a.client.GetTorrents(titleID, filter); err != nil {
		logEv("torrents err: " + err.Error())
	} else {
		res.Torrents = torrents.Torrents
		logEv(fmt.Sprintf("torrents: %d (charged=%.3f€)", len(torrents.Torrents), torrents.Charged))
	}
	if api.LastRawTorrents != "" {
		raw := api.LastRawTorrents
		if len(raw) > 500 {
			raw = raw[:500] + "…"
		}
		logEv("torrents raw: " + raw)
	}
	return res, nil
}

// --- Admin : Mes uploads (list + delete + update) ---

// ListMyLiens retourne les DDL du user courant (via /admin/liens filtré).
func (a *App) ListMyLiens(username string, page int) (*api.AdminLiensResponse, error) {
	a.resetCancellation()
	return a.client.ListAdminLiens(username, page)
}

// ListMyTorrents retourne les torrents du user courant.
func (a *App) ListMyTorrents(username string, page int) (*api.AdminTorrentsResponse, error) {
	a.resetCancellation()
	return a.client.ListAdminTorrents(username, page)
}

// DeleteMyLien — DELETE sur /liens/{id}.
func (a *App) DeleteMyLien(id int) error {
	a.resetCancellation()
	return a.client.DeleteLien(id)
}

// DeleteMyTorrent — DELETE sur /torrents/{id}.
func (a *App) DeleteMyTorrent(id int) error {
	a.resetCancellation()
	return a.client.DeleteTorrent(id)
}

// DeleteMyNzb — DELETE sur /nzb/{id}.
func (a *App) DeleteMyNzb(id int) error {
	a.resetCancellation()
	return a.client.DeleteNzb(id)
}

// NexumIndexEntry : un torrent Nexum + ses clés de lookup.
type NexumIndexEntry struct {
	nexum.Torrent
}

// NexumIndex : map clé → torrent Nexum. Les clés incluent :
//  - info_hash lowercase (rarement utile : les 2 trackers recalculent le hash
//    différemment car ils remplacent le tracker + forcent private=1)
//  - nom normalisé (lowercase, sans espaces/points/underscores) → match robuste
type NexumIndex map[string]nexum.Torrent

// normalizeName : strip . _ - espaces et lower → compare robuste entre
// "Movie.2024.1080p-TEAM" et "Movie 2024 1080p-TEAM" etc.
func normalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// GetNexumIndex : retourne tous les torrents Nexum indexés à la fois par
// info_hash ET par nom normalisé pour matcher malgré le recalcul de hash.
func (a *App) GetNexumIndex() (NexumIndex, error) {
	if a.cfg.NexumApiKey == "" {
		return nil, fmt.Errorf("clé API Nexum manquante")
	}
	c := nexum.NewClient(a.cfg.NexumApiKey, a.cfg.NexumBaseURL)
	torrents, err := c.ListAll()
	if err != nil {
		return nil, err
	}
	idx := make(NexumIndex, len(torrents)*2)
	for _, t := range torrents {
		if t.InfoHash != "" {
			idx["h:"+strings.ToLower(t.InfoHash)] = t
		}
		if t.Name != "" {
			idx["n:"+normalizeName(t.Name)] = t
		}
	}
	return idx, nil
}

// TestNexum : ping /api/v1/me pour vérifier que la clé est valide.
func (a *App) TestNexum() (string, error) {
	if a.cfg.NexumApiKey == "" {
		return "", fmt.Errorf("clé API Nexum manquante")
	}
	c := nexum.NewClient(a.cfg.NexumApiKey, a.cfg.NexumBaseURL)
	info, err := c.Me()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("✓ %s (%s) — ratio %.2f", info.Username, info.Role, info.Ratio), nil
}

// ListSeedboxHashes : retourne tous les info_hash (lowercase) que l'user
// a actuellement sur sa seedbox. Tente ruTorrent d'abord, qBit ensuite.
// Utilisé par Check Torrent pour ne montrer que les torrents que l'user
// a réellement encore en seed (filtre la liste Hydracker).
func (a *App) ListSeedboxHashes() ([]string, error) {
	// 1. ruTorrent (admin)
	if a.cfg.SeedboxURL != "" {
		torrents, err := rutorrent.List(a.cfg.SeedboxURL, a.cfg.SeedboxUser, a.cfg.SeedboxPassword)
		if err == nil {
			out := make([]string, 0, len(torrents))
			for _, t := range torrents {
				if t.Hash != "" {
					out = append(out, strings.ToLower(t.Hash))
				}
			}
			return out, nil
		}
	}
	// 2. qBit (modo)
	if a.cfg.QBitURL != "" {
		hashes, err := qbittorrent.ListHashes(a.cfg.QBitURL, a.cfg.QBitUser, a.cfg.QBitPassword)
		if err == nil {
			return hashes, nil
		}
	}
	return nil, fmt.Errorf("aucune seedbox configurée (ruTorrent ou qBit)")
}

// DeleteTorrentResult — résultat de DeleteTorrentAndFTP, détail par étape.
type DeleteTorrentResult struct {
	HydrackerOK     bool     `json:"hydracker_ok"`
	HydrackerErr    string   `json:"hydracker_err,omitempty"`
	SeedboxOK       bool     `json:"seedbox_ok"`
	SeedboxErr      string   `json:"seedbox_err,omitempty"`
	UsedSeedbox     string   `json:"used_seedbox"` // "rutorrent" | "qbit" | ""
	FTPDeleted      []string `json:"ftp_deleted"`
	FTPErrors       []string `json:"ftp_errors"`
	FilesAttempted  []string `json:"files_attempted"`
	UsedFTP         string   `json:"used_ftp"` // "perso", "mod" ou "" si rien trouvé
}

// DeleteTorrentAndFTP : suppression complète d'un torrent.
//   1. Récupère le .torrent via /torrents/{id}/download (Bearer auth)
//   2. Parse metainfo pour extraire la liste des fichiers
//   3. Tente DELE sur le FTP perso, puis sur le FTP mod si échec
//   4. DELETE /torrents/{id} sur Hydracker
// Idempotent : un fichier déjà absent (550) est traité comme succès.
func (a *App) DeleteTorrentAndFTP(torrentID int) (*DeleteTorrentResult, error) {
	result := &DeleteTorrentResult{FTPDeleted: []string{}, FTPErrors: []string{}, FilesAttempted: []string{}}

	// 1. Récupère le .torrent
	torrentURL := fmt.Sprintf("%s/torrents/%d/download", a.client.BaseURL(), torrentID)
	req, _ := http.NewRequest("GET", torrentURL, nil)
	if a.cfg.HydrackerToken != "" {
		req.Header.Set("Authorization", "Bearer "+a.cfg.HydrackerToken)
	}
	req.Header.Set("User-Agent", "GoPostTools/4.x")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result, fmt.Errorf("download torrent: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return result, fmt.Errorf("download torrent HTTP %d", resp.StatusCode)
	}
	tmpFile, err := os.CreateTemp("", "del-*.torrent")
	if err != nil {
		return result, err
	}
	defer os.Remove(tmpFile.Name())
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return result, err
	}
	tmpFile.Close()

	// 2. Parse metainfo → liste des fichiers
	mi, err := metainfo.LoadFromFile(tmpFile.Name())
	if err != nil {
		return result, fmt.Errorf("parse .torrent: %w", err)
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return result, fmt.Errorf("unmarshal info: %w", err)
	}
	if len(info.Files) == 0 {
		// Single-file torrent
		result.FilesAttempted = []string{info.Name}
	} else {
		for _, f := range info.Files {
			result.FilesAttempted = append(result.FilesAttempted, f.DisplayPath(&info))
		}
	}

	// 2.5. Supprime le torrent de la seedbox (ruTorrent puis qBit en fallback)
	infoHash := strings.ToLower(mi.HashInfoBytes().HexString())
	if a.cfg.SeedboxURL != "" {
		if err := rutorrent.Erase(a.cfg.SeedboxURL, a.cfg.SeedboxUser, a.cfg.SeedboxPassword, infoHash); err == nil {
			result.SeedboxOK = true
			result.UsedSeedbox = "rutorrent"
		} else {
			result.SeedboxErr = "rutorrent: " + err.Error()
		}
	}
	if !result.SeedboxOK && a.cfg.QBitURL != "" {
		if err := qbittorrent.Delete(a.cfg.QBitURL, a.cfg.QBitUser, a.cfg.QBitPassword, infoHash, false); err == nil {
			result.SeedboxOK = true
			result.UsedSeedbox = "qbit"
			result.SeedboxErr = ""
		} else {
			if result.SeedboxErr != "" {
				result.SeedboxErr += " · "
			}
			result.SeedboxErr += "qbit: " + err.Error()
		}
	}

	// 3. Tente FTP perso d'abord, puis FTP mod en fallback
	tryFTP := func(host string, port int, user, password, path, label string) bool {
		if host == "" || user == "" {
			return false
		}
		anyOK := false
		for _, fname := range result.FilesAttempted {
			err := ftpup.Delete(host, port, user, password, path, fname)
			if err == nil {
				result.FTPDeleted = append(result.FTPDeleted, fname+" ("+label+")")
				anyOK = true
			} else {
				result.FTPErrors = append(result.FTPErrors, fname+" ("+label+"): "+err.Error())
			}
		}
		return anyOK
	}
	if tryFTP(a.cfg.FTPHost, a.cfg.FTPPort, a.cfg.FTPUser, a.cfg.FTPPassword, a.cfg.FTPPath, "perso") {
		result.UsedFTP = "perso"
	} else if tryFTP(a.cfg.FTPModHost, a.cfg.FTPModPort, a.cfg.FTPModUser, a.cfg.FTPModPassword, a.cfg.FTPModPath, "mod") {
		result.UsedFTP = "mod"
	}

	// 4. DELETE Hydracker
	if err := a.client.DeleteTorrent(torrentID); err != nil {
		result.HydrackerErr = err.Error()
		return result, fmt.Errorf("delete hydracker: %w", err)
	}
	result.HydrackerOK = true
	return result, nil
}

// UpdateMyLien — PUT sur /liens/{id} avec les champs modifiables.
// Passer 0 pour laisser un champ inchangé (omitempty côté JSON).
// activeValue : -1 = ne pas toucher, 0 = désactiver, 1 = activer
func (a *App) UpdateMyLien(id, quality, lang, saison, episode, activeValue int) error {
	a.resetCancellation()
	p := api.UpdateLienPayload{
		Quality: quality,
		Lang:    lang,
		Season:  saison,
		Episode: episode,
	}
	if activeValue >= 0 {
		v := activeValue
		p.Active = &v
	}
	return a.client.UpdateLien(id, p)
}

// UpdateMyTorrent — PUT sur /torrents/{id}.
func (a *App) UpdateMyTorrent(id, quality, lang, saison, episode, activeValue int) error {
	a.resetCancellation()
	p := api.UpdateTorrentPayload{
		Quality: quality,
		Lang:    lang,
		Season:  saison,
		Episode: episode,
	}
	if activeValue >= 0 {
		v := activeValue
		p.Active = &v
	}
	return a.client.UpdateTorrent(id, p)
}

// ListTitlesSorted expose /titles avec tri + pagination pour l'onglet Stats.
// order : "popularity:desc" | "score:desc" | "release_date:desc" | ...
func (a *App) ListTitlesSorted(order string, perPage, page int) (*api.TitlesResponse, error) {
	a.resetCancellation()
	if perPage <= 0 {
		perPage = 20
	}
	if page <= 0 {
		page = 1
	}
	f := api.TitleFilter{
		PerPage: perPage,
		Page:    page,
		Order:   order,
	}
	return a.client.GetTitles(f)
}

// ListReseedRequests expose l'endpoint admin /reseed-requests au frontend.
// Filtres optionnels : status ("pending"|"done"|"rejected"|""),
// uploaderID/requesterID (0 pour ignorer), page (1+).
func (a *App) ListReseedRequests(status string, uploaderID, requesterID, page int) (*api.ReseedRequestsResponse, error) {
	a.resetCancellation()
	return a.client.ListReseedRequests(status, uploaderID, requesterID, page)
}

// AutoReseedResult retourne le résultat du workflow auto-reseed.
type AutoReseedResult struct {
	TorrentID    int    `json:"torrent_id"`
	TorrentName  string `json:"torrent_name"`
	Quality      int    `json:"quality"`
	Lang         int    `json:"lang"`
	Season       int    `json:"saison"`
	Episode      int    `json:"episode"`
	SizeBytes    int64  `json:"size_bytes"`
	SeedboxPath  string `json:"seedbox_path"`
}

// pickBestTorrent choisit le meilleur torrent parmi la liste.
// Priorité : match exact qualité demandée > TrueFrench (8) > French (Canada) (7) > autres
// À qualité/lang égales : le plus récent (ID le plus haut).
func pickBestTorrent(torrents []api.TorrentItem, preferQuality, preferLang int) *api.TorrentItem {
	if len(torrents) == 0 {
		return nil
	}
	score := func(t api.TorrentItem) int {
		s := 0
		if preferQuality > 0 && t.Quality == preferQuality {
			s += 1000
		}
		tLang := t.PrimaryLangID()
		if preferLang > 0 && tLang == preferLang {
			s += 500
		} else {
			switch tLang {
			case 8:
				s += 100 // TrueFrench
			case 7:
				s += 80 // French (Canada)
			case 5:
				s += 40 // English
			}
		}
		// Bonus qualité 1080p par défaut si pas de préférence
		if preferQuality == 0 {
			switch t.Quality {
			case 52, 50, 17: // HD 1080p, HDLight 1080p, Blu-Ray 1080p
				s += 60
			case 60: // ULTRA HDLight
				s += 50
			case 31: // HD 720p
				s += 30
			}
		}
		// Priorise les ID récents à score équivalent
		s += t.ID / 10000
		return s
	}
	best := &torrents[0]
	bestScore := score(torrents[0])
	for i := 1; i < len(torrents); i++ {
		if sc := score(torrents[i]); sc > bestScore {
			bestScore = sc
			best = &torrents[i]
		}
	}
	return best
}

// AutoReseedFromHydracker automatise un reseed : liste les torrents dispo pour
// une fiche (+ saison/épisode), choisit le meilleur match, récupère son URL de
// download via /content/torrents/{id}, télécharge le .torrent et le push direct
// sur la seedbox ruTorrent. Aucun MKV / FTP impliqué — on se branche sur un
// torrent déjà distribué par le site.
//
// preferQuality / preferLang : IDs optionnels pour forcer un choix précis.
// Passer 0 laisse l'algorithme choisir (FR + 1080p prioritaire).
func (a *App) AutoReseedFromHydracker(titleID, saison, episode, preferQuality, preferLang int) (*AutoReseedResult, error) {
	a.resetCancellation()
	if titleID <= 0 {
		return nil, fmt.Errorf("title_id manquant")
	}
	if a.cfg.SeedboxURL == "" {
		return nil, fmt.Errorf("seedbox non configurée (Réglages)")
	}
	emit := func(stage, msg string) {
		wailsruntime.EventsEmit(a.ctx, "autoreseed:status", map[string]interface{}{"stage": stage, "msg": msg})
	}

	// 1. Liste les torrents
	emit("list", fmt.Sprintf("Recherche torrents pour fiche #%d…", titleID))
	filter := api.ContentFilter{Season: saison, Episode: episode}
	if preferQuality > 0 {
		filter.Quality = preferQuality
	}
	if preferLang > 0 {
		filter.Lang = preferLang
	}
	res, err := a.client.GetTorrents(titleID, filter)
	if err != nil {
		// Fallback 1 : retry sans saison/épisode si 500 (parfois le serveur plante sur les filtres)
		emit("list", fmt.Sprintf("torrents err: %v — retry sans saison/épisode", err))
		fb, err2 := a.client.GetTorrents(titleID, api.ContentFilter{})
		if err2 != nil {
			return nil, fmt.Errorf("liste torrents : %w (pas de torrent partagé via API pour cette fiche ?)", err)
		}
		res = fb
	}
	if len(res.Torrents) == 0 && (preferQuality > 0 || preferLang > 0) {
		emit("list", "Aucun résultat avec filtres qual/lang — retry sans")
		fb, err := a.client.GetTorrents(titleID, api.ContentFilter{Season: saison, Episode: episode})
		if err == nil {
			res = fb
		}
	}
	if len(res.Torrents) == 0 {
		return nil, fmt.Errorf("aucun torrent partagé via API pour fiche #%d — essayez l'auto-reseed DDL si un lien DDL est dispo", titleID)
	}
	emit("list_done", fmt.Sprintf("%d torrent(s) dispo", len(res.Torrents)))

	// 2. Choix du meilleur
	best := pickBestTorrent(res.Torrents, preferQuality, preferLang)
	if best == nil {
		return nil, fmt.Errorf("aucun torrent sélectionnable")
	}
	emit("pick", fmt.Sprintf("Choisi : #%d %s (qual=%d lang=%d %.2f GB)", best.ID, best.Name, best.Quality, best.PrimaryLangID(), float64(best.Size)/1e9))

	// 3. Télécharge le .torrent via l'endpoint API /api/v1/torrents/{id}/download
	// (Bearer-authentifié, renvoie du bencode direct).
	// Ne PAS utiliser le download_url retourné par /content/torrents/{id} : c'est
	// une URL web signée qui attend une session cookie et renvoie le shell HTML
	// de l'app frontend si on y accède sans cookie.
	emit("download", "Téléchargement .torrent…")
	data, err := a.downloadHydrackerTorrent(best.ID)
	if err != nil {
		return nil, fmt.Errorf("download torrent #%d : %w", best.ID, err)
	}
	if len(data) < 50 || data[0] != 'd' {
		preview := string(data)
		if len(preview) > 200 {
			preview = preview[:200] + "…"
		}
		return nil, fmt.Errorf("fichier reçu n'est pas un .torrent bencode valide (%d bytes, début=%q)", len(data), preview)
	}
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("autoreseed-%d-*.torrent", best.ID))
	if err != nil {
		return nil, fmt.Errorf("tmp file : %w", err)
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("write tmp : %w", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpPath)
	emit("download_done", fmt.Sprintf("Téléchargé (%.2f KB)", float64(len(data))/1024))

	// 5. Push sur seedbox
	emit("seedbox", "Upload sur seedbox…")
	seedPath, err := a.pushTorrent(a.workContext(), tmpPath, "", func(p seedbox.Progress) {
		wailsruntime.EventsEmit(a.ctx, "autoreseed:progress", p)
	})
	if err != nil {
		return nil, fmt.Errorf("seedbox : %w", err)
	}
	emit("done", fmt.Sprintf("Seedbox OK : %s", seedPath))

	return &AutoReseedResult{
		TorrentID:   best.ID,
		TorrentName: best.Name,
		Quality:     best.Quality,
		Lang:        best.PrimaryLangID(),
		Season:      best.Season,
		Episode:     best.Episode,
		SizeBytes:   best.Size,
		SeedboxPath: seedPath,
	}, nil
}

// AutoReseedDDLResult retourne le résultat du workflow DDL auto-reseed.
type AutoReseedDDLResult struct {
	LienID      int    `json:"lien_id"`
	Filename    string `json:"filename"`
	Host        string `json:"host"`
	SizeBytes   int64  `json:"size_bytes"`
	FTPRemoteName string `json:"ftp_remote_name"`
}

// pickBestLien choisit le meilleur DDL parmi la liste. Priorité : 1fichier
// (on a l'API key), puis autres. À hosts égaux : même logique quali/lang.
func pickBestLien(liens []api.Lien, preferQuality, preferLang int) *api.Lien {
	if len(liens) == 0 {
		return nil
	}
	score := func(l api.Lien) int {
		s := 0
		h := strings.ToLower(l.HostName())
		if strings.Contains(h, "1fichier") {
			s += 1000 // on a l'API key premium
		} else if strings.Contains(h, "send") {
			s += 500
		}
		if preferQuality > 0 && l.Quality == preferQuality {
			s += 300
		}
		lLang := l.PrimaryLangID()
		if preferLang > 0 && lLang == preferLang {
			s += 150
		} else {
			switch lLang {
			case 8:
				s += 100
			case 7:
				s += 80
			case 5:
				s += 40
			}
		}
		if preferQuality == 0 {
			switch l.Quality {
			case 52, 50, 17:
				s += 60
			case 60:
				s += 50
			case 31:
				s += 30
			}
		}
		s += l.ID / 10000
		return s
	}
	best := &liens[0]
	bestScore := score(liens[0])
	for i := 1; i < len(liens); i++ {
		if sc := score(liens[i]); sc > bestScore {
			bestScore = sc
			best = &liens[i]
		}
	}
	return best
}

// AutoReseedDDLFromHydracker télécharge depuis un DDL Hydracker (1fichier
// prioritaire) et stream direct vers le FTP configuré — pas de passage sur
// le disque local. Utile quand aucun torrent n'est partagé via API mais qu'un
// DDL existe.
//
// Workflow :
//  1. Liste les DDL via /content/liens (+saison/épisode)
//  2. Picke le meilleur (1fichier préféré, puis qualité/langue)
//  3. Récupère l'URL directe via /content/liens/{id}
//  4. Si 1fichier : API /download/get_token.cgi → URL directe
//  5. HTTP GET streaming → FTP UploadFromReader
//
// Prérequis : API key 1fichier + FTP configurés dans Réglages.
func (a *App) AutoReseedDDLFromHydracker(titleID, saison, episode, preferQuality, preferLang int) (*AutoReseedDDLResult, error) {
	a.resetCancellation()
	if titleID <= 0 {
		return nil, fmt.Errorf("title_id manquant")
	}
	if a.cfg.FTPHost == "" {
		return nil, fmt.Errorf("FTP non configuré (Réglages)")
	}
	emit := func(stage, msg string) {
		wailsruntime.EventsEmit(a.ctx, "autoreseed_ddl:status", map[string]interface{}{"stage": stage, "msg": msg})
	}

	// 1. Liste les liens
	emit("list", fmt.Sprintf("Recherche DDL pour fiche #%d…", titleID))
	filter := api.ContentFilter{Season: saison, Episode: episode}
	if preferQuality > 0 {
		filter.Quality = preferQuality
	}
	if preferLang > 0 {
		filter.Lang = preferLang
	}
	res, err := a.client.GetLiens(titleID, filter)
	if err != nil {
		emit("list", fmt.Sprintf("liens err: %v — retry sans filtres", err))
		fb, err2 := a.client.GetLiens(titleID, api.ContentFilter{})
		if err2 != nil {
			return nil, fmt.Errorf("liste liens : %w", err)
		}
		res = fb
	}
	if len(res.Liens) == 0 && (preferQuality > 0 || preferLang > 0) {
		emit("list", "Aucun DDL avec filtres — retry sans")
		if fb, err := a.client.GetLiens(titleID, api.ContentFilter{Season: saison, Episode: episode}); err == nil {
			res = fb
		}
	}
	if len(res.Liens) == 0 {
		return nil, fmt.Errorf("aucun DDL trouvé pour fiche #%d saison=%d épisode=%d", titleID, saison, episode)
	}
	emit("list_done", fmt.Sprintf("%d DDL(s) dispo", len(res.Liens)))

	// 2. Choix
	best := pickBestLien(res.Liens, preferQuality, preferLang)
	if best == nil {
		return nil, fmt.Errorf("aucun DDL sélectionnable")
	}
	emit("pick", fmt.Sprintf("Choisi : #%d host=%s qual=%d lang=%d", best.ID, best.HostName(), best.Quality, best.PrimaryLangID()))

	// 3. Récupère URL partage (via /content/liens/{id} qui retourne le champ "lien")
	emit("url", "Récupération URL DDL…")
	lienData, err := a.client.GetLienByID(best.ID)
	if err != nil {
		return nil, fmt.Errorf("get lien #%d : %w", best.ID, err)
	}
	if lienData == nil || lienData.URL == "" {
		return nil, fmt.Errorf("lien manquant pour DDL #%d", best.ID)
	}
	shareURL := lienData.URL

	// 4. Pour 1fichier : get direct URL via API
	directURL := shareURL
	host := strings.ToLower(lienData.HostName())
	if strings.Contains(host, "1fichier") || strings.Contains(shareURL, "1fichier.com") {
		if a.cfg.OneFichierApiKey == "" {
			return nil, fmt.Errorf("DDL 1fichier trouvé mais clé API 1fichier non configurée (Réglages)")
		}
		emit("token", "Obtention URL directe 1fichier…")
		direct, err := downloader.OneFichierGetToken(a.workContext(), a.cfg.OneFichierApiKey, shareURL)
		if err != nil {
			return nil, fmt.Errorf("1fichier token : %w", err)
		}
		directURL = direct
	} else {
		return nil, fmt.Errorf("host %s non supporté en auto (seul 1fichier l'est pour l'instant) — URL: %s", lienData.HostName(), shareURL)
	}

	// 5. Stream download → FTP
	emit("download", "Ouverture du stream…")
	reader, totalSize, hdrFilename, err := downloader.StreamDownload(a.workContext(), directURL)
	if err != nil {
		return nil, fmt.Errorf("download stream : %w", err)
	}
	defer reader.Close()

	filename := hdrFilename
	if filename == "" {
		// Fallback : utilise le nom du lien ou extrait de l'URL
		filename = fmt.Sprintf("hydracker-%d.mkv", best.ID)
	}

	emit("ftp", fmt.Sprintf("Upload FTP : %s (%.2f GB)", filename, float64(totalSize)/1e9))
	progReader := &downloader.ProgressReader{
		R:     reader,
		Total: totalSize,
		OnProgress: func(p downloader.Progress) {
			wailsruntime.EventsEmit(a.ctx, "autoreseed_ddl:progress", p)
		},
	}
	if err := ftpup.UploadFromReader(a.workContext(), a.cfg.FTPHost, a.cfg.FTPPort, a.cfg.FTPUser, a.cfg.FTPPassword, a.cfg.FTPPath, filename, progReader, totalSize, nil); err != nil {
		return nil, fmt.Errorf("ftp : %w", err)
	}
	emit("done", fmt.Sprintf("FTP OK : %s", filename))

	return &AutoReseedDDLResult{
		LienID:        best.ID,
		Filename:      filename,
		Host:          lienData.HostName(),
		SizeBytes:     totalSize,
		FTPRemoteName: filename,
	}, nil
}

// AutoReseedFullResult : résultat du workflow "reseed complet" qui combine
// DDL→FTP + push .torrent seedbox + force recheck. C'est le flow idéal pour
// traiter une demande de reseed où un seul torrent existe mais il y a un DDL
// dispo comme source alternative.
type AutoReseedFullResult struct {
	TorrentID        int    `json:"torrent_id"`
	TorrentName      string `json:"torrent_name"`
	ExpectedFilename string `json:"expected_filename"`
	MatchedLienID    int    `json:"matched_lien_id"`
	MatchedHost      string `json:"matched_host"`
	SizeBytes        int64  `json:"size_bytes"`
	InfoHash         string `json:"info_hash"`
	SeedboxPath      string `json:"seedbox_path"`
	Rechecked        bool   `json:"rechecked"`
}

// AutoReseedFullFromTorrent : workflow complet pour une demande de reseed.
//  1. Download .torrent via /api/v1/torrents/{id}/download (Bearer)
//  2. Parse metainfo → extrait le nom exact du fichier attendu (single-file only)
//  3. Cherche le meilleur DDL dispo sur la fiche (1fichier prioritaire)
//  4. 1fichier get_token → URL directe
//  5. Stream download DDL → FTP upload sous le nom EXACT du torrent
//  6. Push .torrent à ruTorrent (addtorrent.php)
//  7. Sleep 2s puis force recheck (d.check_hash) → rtorrent commence à seed
//
// Multi-file torrents : non supportés pour l'instant (besoin de mapper les
// fichiers du DDL vers ceux du torrent, cas rare).
func (a *App) AutoReseedFullFromTorrent(torrentID, titleID, saison, episode int) (*AutoReseedFullResult, error) {
	a.resetCancellation()
	if torrentID <= 0 || titleID <= 0 {
		return nil, fmt.Errorf("torrent_id et title_id requis")
	}
	if a.cfg.SeedboxURL == "" {
		return nil, fmt.Errorf("seedbox non configurée (Réglages)")
	}
	if a.cfg.FTPHost == "" {
		return nil, fmt.Errorf("FTP non configuré (Réglages)")
	}
	if a.cfg.OneFichierApiKey == "" {
		return nil, fmt.Errorf("clé API 1fichier non configurée (Réglages)")
	}
	emit := func(stage, msg string) {
		wailsruntime.EventsEmit(a.ctx, "autoreseed_full:status", map[string]interface{}{"stage": stage, "msg": msg})
	}

	// 1. Download .torrent via API
	emit("torrent_dl", fmt.Sprintf("Téléchargement .torrent #%d…", torrentID))
	torrentData, err := a.downloadHydrackerTorrent(torrentID)
	if err != nil {
		return nil, fmt.Errorf("download torrent : %w", err)
	}
	if len(torrentData) < 50 || torrentData[0] != 'd' {
		return nil, fmt.Errorf("torrent invalide (%d bytes)", len(torrentData))
	}

	// 2. Sauvegarde temp + parse metainfo
	tmpTor, err := os.CreateTemp("", fmt.Sprintf("reseedfull-%d-*.torrent", torrentID))
	if err != nil {
		return nil, fmt.Errorf("tmp torrent : %w", err)
	}
	tmpTorPath := tmpTor.Name()
	if _, err := tmpTor.Write(torrentData); err != nil {
		tmpTor.Close()
		os.Remove(tmpTorPath)
		return nil, fmt.Errorf("write tmp torrent : %w", err)
	}
	tmpTor.Close()
	defer os.Remove(tmpTorPath)

	mi, err := metainfo.LoadFromFile(tmpTorPath)
	if err != nil {
		return nil, fmt.Errorf("parse metainfo : %w", err)
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return nil, fmt.Errorf("unmarshal info : %w", err)
	}
	if len(info.Files) > 0 {
		return nil, fmt.Errorf("torrent multi-files non supporté (info.Files count=%d) — pour l'instant seuls les single-file sont pris en charge", len(info.Files))
	}
	expectedFilename := info.Name
	infoHash := mi.HashInfoBytes().HexString()
	emit("parsed", fmt.Sprintf("Fichier attendu : %s (%.2f GB)", expectedFilename, float64(info.Length)/1e9))

	// 3. Cherche le meilleur DDL sur la fiche (mêmes saison/épisode)
	emit("ddl_search", "Recherche DDL 1fichier…")
	liensResp, err := a.client.GetLiens(titleID, api.ContentFilter{Season: saison, Episode: episode})
	if err != nil {
		return nil, fmt.Errorf("liste DDL : %w", err)
	}
	if len(liensResp.Liens) == 0 {
		// Retry sans saison/épisode
		if fb, err := a.client.GetLiens(titleID, api.ContentFilter{}); err == nil {
			liensResp = fb
		}
	}
	if len(liensResp.Liens) == 0 {
		return nil, fmt.Errorf("aucun DDL dispo sur la fiche #%d", titleID)
	}

	// 4. Itère les DDL 1fichier dans l'ordre de préférence. Si le premier
	//    est mort ("Resource not found"), passe au suivant. Fail uniquement
	//    si tous les candidats échouent.
	//    On filtre + trie : 1fichier prioritaire, qualité décroissante.
	candidates := []api.Lien{}
	for _, l := range liensResp.Liens {
		host := strings.ToLower(l.HostName())
		if strings.Contains(host, "1fichier") || l.IDHost == 5 {
			candidates = append(candidates, l)
		}
	}
	// Top-1 via pickBestLien en tête, puis le reste
	if top := pickBestLien(liensResp.Liens, 0, 0); top != nil {
		// Mets le meilleur en tête, retire-le du reste
		reordered := []api.Lien{*top}
		for _, l := range candidates {
			if l.ID != top.ID {
				reordered = append(reordered, l)
			}
		}
		candidates = reordered
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("aucun DDL 1fichier sur la fiche #%d", titleID)
	}

	var directURL string
	var pickedLien api.Lien
	var pickedHost string
	var tokenErrors []string
	for _, candidate := range candidates {
		emit("ddl_picked", fmt.Sprintf("DDL essayé : #%d", candidate.ID))
		lienData, err := a.client.GetLienByID(candidate.ID)
		if err != nil || lienData == nil || lienData.URL == "" {
			tokenErrors = append(tokenErrors, fmt.Sprintf("#%d get: %v", candidate.ID, err))
			continue
		}
		emit("token", fmt.Sprintf("Obtention URL directe #%d…", candidate.ID))
		d, terr := downloader.OneFichierGetToken(a.workContext(), a.cfg.OneFichierApiKey, lienData.URL)
		if terr != nil {
			tokenErrors = append(tokenErrors, fmt.Sprintf("#%d: %v", candidate.ID, terr))
			emit("ddl_skip", fmt.Sprintf("#%d mort (%v) — passe au suivant", candidate.ID, terr))
			continue
		}
		directURL = d
		pickedLien = candidate
		pickedHost = lienData.HostName()
		break
	}
	if directURL == "" {
		return nil, fmt.Errorf("aucun DDL 1fichier valide (%d testés) : %s", len(candidates), strings.Join(tokenErrors, " · "))
	}

	// 5. Stream download → FTP avec le nom EXACT du torrent
	emit("download", "Ouverture du stream…")
	reader, totalSize, _, err := downloader.StreamDownload(a.workContext(), directURL)
	if err != nil {
		return nil, fmt.Errorf("download stream : %w", err)
	}
	defer reader.Close()

	emit("ftp", fmt.Sprintf("Upload FTP : %s (%.2f GB)", expectedFilename, float64(totalSize)/1e9))
	progReader := &downloader.ProgressReader{
		R:     reader,
		Total: totalSize,
		OnProgress: func(p downloader.Progress) {
			wailsruntime.EventsEmit(a.ctx, "autoreseed_full:progress", p)
		},
	}
	if err := ftpup.UploadFromReader(a.workContext(), a.cfg.FTPHost, a.cfg.FTPPort, a.cfg.FTPUser, a.cfg.FTPPassword, a.cfg.FTPPath, expectedFilename, progReader, totalSize, nil); err != nil {
		return nil, fmt.Errorf("ftp : %w", err)
	}
	emit("ftp_done", fmt.Sprintf("FTP OK : %s", expectedFilename))

	// 6. Push .torrent à ruTorrent
	emit("seedbox", "Ajout du .torrent sur ruTorrent…")
	seedPath, err := a.pushTorrent(a.workContext(), tmpTorPath, "", func(p seedbox.Progress) {
		wailsruntime.EventsEmit(a.ctx, "autoreseed_full:seedbox", p)
	})
	if err != nil {
		return nil, fmt.Errorf("seedbox : %w", err)
	}

	// 7. Sleep 2s + force recheck (rtorrent a besoin d'un moment pour enregistrer)
	emit("recheck", "Force recheck…")
	time.Sleep(2 * time.Second)
	rechecked := true
	if err := a.recheckTorrent(infoHash, ""); err != nil {
		emit("recheck_warn", fmt.Sprintf("recheck auto échoué (%v) — à faire manuellement si besoin", err))
		rechecked = false
	}
	emit("done", "Reseed complet terminé")

	return &AutoReseedFullResult{
		TorrentID:        torrentID,
		TorrentName:      info.Name,
		ExpectedFilename: expectedFilename,
		MatchedLienID:    pickedLien.ID,
		MatchedHost:      pickedHost,
		SizeBytes:        totalSize,
		InfoHash:         infoHash,
		SeedboxPath:      seedPath,
		Rechecked:        rechecked,
	}, nil
}

// downloadFile télécharge une URL (avec token Bearer si c'est Hydracker) et
// retourne le contenu brut. Utilisé pour les .torrent récupérés via l'API
// /content/torrents/{id}.
func (a *App) downloadFile(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	// Set Bearer pour TOUT le domaine Hydracker (storage/torrents/, api/v1/, etc.),
	// pas juste pour /api/v1. Détection : extraction du host depuis effectiveHydrackerURL.
	hydHost := ""
	if hURL := effectiveHydrackerURL(a.cfg); hURL != "" {
		// hURL = "https://hydracker.com/api/v1" → host = "hydracker.com"
		hydHost = strings.TrimPrefix(strings.TrimPrefix(hURL, "https://"), "http://")
		if i := strings.Index(hydHost, "/"); i != -1 {
			hydHost = hydHost[:i]
		}
	}
	if hydHost != "" && strings.Contains(url, hydHost) && a.cfg.HydrackerToken != "" {
		req.Header.Set("Authorization", "Bearer "+a.cfg.HydrackerToken)
	}
	// UA descriptif requis par le WAF Hydracker (et safe pour autres hosts)
	req.Header.Set("User-Agent", "GoPostTools/3.0 (https://github.com/Gandalfleblanc/Go-Post-Tools)")
	c := &http.Client{Timeout: 120 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		preview := string(body)
		if len(preview) > 300 {
			preview = preview[:300] + "…"
		}
		return nil, fmt.Errorf("HTTP %d sur %s: %s", resp.StatusCode, url, preview)
	}
	return io.ReadAll(resp.Body)
}

// PostExistingTorrent poste un .torrent déjà existant à Hydracker (sans FTP ni seedbox).
// Utilisé quand l'utilisateur n'a pas de seedbox ou garde son MKV sur un NAS/local.
func (a *App) PostExistingTorrent(titleID, qualite int, langues, subs []string, torrentPath, nfo string, saison, episode int) (*TorrentWorkflowResult, error) {
	a.resetCancellation()
	if strings.TrimSpace(torrentPath) == "" {
		return nil, fmt.Errorf("chemin .torrent manquant")
	}
	wailsruntime.EventsEmit(a.ctx, "torrent:status", map[string]interface{}{"stage": "post", "msg": "Post du .torrent sur Hydracker…"})
	uploaded, err := a.client.UploadTorrent(titleID, qualite, langues, subs, torrentPath, nfo, saison, episode)
	if err != nil {
		return nil, fmt.Errorf("hydracker: %w", err)
	}
	wailsruntime.EventsEmit(a.ctx, "torrent:status", map[string]interface{}{"stage": "done", "msg": fmt.Sprintf("Torrent #%d ajouté", uploaded.Torrent.ID)})
	a.recordHistory(history.Entry{
		Type:        "torrent",
		TitleID:     titleID,
		TitleName:   a.titleName(titleID),
		Saison:      saison,
		Episode:     episode,
		Qualite:     qualite,
		QualiteName: a.qualiteName(qualite),
		HydrackerID: uploaded.Torrent.ID,
		Filename:    filepath.Base(torrentPath),
		Status:      "ok",
	})
	return &TorrentWorkflowResult{
		TorrentPath: torrentPath,
		HydrackerID: uploaded.Torrent.ID,
	}, nil
}

// PostTorrentWorkflow : post complet torrent.
// seedboxType : "admin" (ruTorrent) | "modo" (qBit) | "" (auto : qBit si configuré sinon ruTorrent).
func (a *App) PostTorrentWorkflow(titleID, qualite int, langues, subs []string, mkvPath, nfo string, saison, episode int, seedboxType string) (*TorrentWorkflowResult, error) {
	a.resetCancellation()
	if strings.TrimSpace(mkvPath) == "" {
		return nil, fmt.Errorf("chemin MKV manquant")
	}
	// Pré-check config selon le type. Pour "prive" on skip les checks admin
	// (cfg.FTPHost/SeedboxURL) et on vérifie Private*. Le switch FTP/seedbox
	// plus bas fait le check final.
	switch seedboxType {
	case "prive":
		if a.cfg.PrivateFTPHost == "" || a.cfg.PrivateFTPUser == "" {
			return nil, fmt.Errorf("FTP Privé non configuré (Réglages)")
		}
		if a.cfg.PrivateSeedboxURL == "" && a.cfg.PrivateQBitURL == "" {
			return nil, fmt.Errorf("Seedbox Privée non configurée (Réglages : ruTorrent ou qBittorrent)")
		}
	case "modo":
		if a.cfg.FTPModHost == "" {
			return nil, fmt.Errorf("FTP Modérateur non configuré (Réglages)")
		}
		if a.cfg.QBitURL == "" {
			return nil, fmt.Errorf("qBit Modérateur non configuré (Réglages)")
		}
	default: // admin
		if a.cfg.FTPHost == "" || a.cfg.FTPUser == "" {
			return nil, fmt.Errorf("FTP ADMIN non configuré — renseignez les Settings")
		}
		if a.cfg.SeedboxURL == "" {
			return nil, fmt.Errorf("seedbox ADMIN non configurée — renseignez les Settings")
		}
	}
	if a.cfg.TrackerURL == "" {
		return nil, fmt.Errorf("URL tracker manquante — renseignez les Settings")
	}
	pieceSize := int64(a.cfg.TorrentPieceSize)
	if pieceSize <= 0 {
		pieceSize = 8 * 1024 * 1024
	}

	emit := func(stage, msg string) {
		wailsruntime.EventsEmit(a.ctx, "torrent:status", map[string]interface{}{"stage": stage, "msg": msg})
	}

	// 1. Upload MKV vers la seedbox cible via FTP
	//    - MODO  → FTP MODÉRATEUR (cfg.FTPMod*) — seedbox partagée des modos
	//    - PRIVE → FTP perso de l'user (cfg.PrivateFTP*) — saisi en Réglages
	//    - ADMIN → FTP ADMIN (cfg.FTP*) — seedbox team-shared baked
	var ftpHost, ftpUser, ftpPass, ftpPath string
	var ftpPort int
	switch seedboxType {
	case "modo":
		if a.cfg.FTPModHost == "" {
			return nil, fmt.Errorf("FTP Modérateur non configuré (Réglages)")
		}
		ftpHost, ftpPort, ftpUser, ftpPass, ftpPath = a.cfg.FTPModHost, a.cfg.FTPModPort, a.cfg.FTPModUser, a.cfg.FTPModPassword, a.cfg.FTPModPath
		emit("ftp", "Upload FTP Modérateur…")
	case "prive":
		if a.cfg.PrivateFTPHost == "" {
			return nil, fmt.Errorf("FTP Privé non configuré (Réglages)")
		}
		ftpHost, ftpPort, ftpUser, ftpPass, ftpPath = a.cfg.PrivateFTPHost, a.cfg.PrivateFTPPort, a.cfg.PrivateFTPUser, a.cfg.PrivateFTPPassword, a.cfg.PrivateFTPPath
		emit("ftp", "Upload FTP Privé…")
	default: // "admin" ou ""
		ftpHost, ftpPort, ftpUser, ftpPass, ftpPath = a.cfg.FTPHost, a.cfg.FTPPort, a.cfg.FTPUser, a.cfg.FTPPassword, a.cfg.FTPPath
		emit("ftp", "Upload FTP…")
	}
	remoteName, err := ftpup.Upload(a.workContext(), ftpHost, ftpPort, ftpUser, ftpPass, ftpPath, mkvPath, func(p ftpup.Progress) {
		wailsruntime.EventsEmit(a.ctx, "torrent:ftp", p)
	})
	if err != nil {
		return nil, fmt.Errorf("ftp: %w", err)
	}
	emit("ftp_done", fmt.Sprintf("FTP OK : %s", remoteName))

	// 2. Créer le .torrent
	ext := filepath.Ext(mkvPath)
	releaseName := strings.TrimSuffix(filepath.Base(mkvPath), ext)
	torrentPath := filepath.Join(filepath.Dir(mkvPath), releaseName+".torrent")
	emit("create", "Création du .torrent…")
	if err := torrent.Create(mkvPath, a.cfg.TrackerURL, torrentPath, pieceSize, func(p torrent.Progress) {
		wailsruntime.EventsEmit(a.ctx, "torrent:create", p)
	}); err != nil {
		return nil, fmt.Errorf("create torrent: %w", err)
	}
	emit("create_done", "Torrent généré")

	if a.isCancelled() {
		return nil, fmt.Errorf("annulé par l'utilisateur")
	}
	// 3a. Dedup : si un retry précédent a créé un torrent avec le même info_hash
	//     ou nom, le supprimer avant de re-uploader (évite l'erreur 422 "duplicate").
	if mi, err := metainfo.LoadFromFile(torrentPath); err == nil {
		newHash := mi.HashInfoBytes().HexString()
		info, _ := mi.UnmarshalInfo()
		if existingID, _ := a.findExistingTorrent(titleID, newHash, info.Name); existingID > 0 {
			emit("dedup", fmt.Sprintf("Suppression du duplicate précédent #%d…", existingID))
			if err := a.client.DeleteTorrent(existingID); err != nil {
				emit("dedup_warn", fmt.Sprintf("dedup échoué : %s (ignoré)", err.Error()))
			} else {
				emit("dedup_done", fmt.Sprintf("Duplicate #%d supprimé", existingID))
			}
		}
	}

	// 3. Post sur Hydracker
	emit("post", "Post sur Hydracker…")
	uploaded, err := a.client.UploadTorrent(titleID, qualite, langues, subs, torrentPath, nfo, saison, episode)
	if err != nil {
		emit("error", fmt.Sprintf("Hydracker : %s", err.Error()))
		return nil, fmt.Errorf("hydracker: %w", err)
	}
	emit("post_done", fmt.Sprintf("Post OK #%d", uploaded.Torrent.ID))

	// 4. Télécharger le .torrent généré par Hydracker
	hydTorPath := ""
	if uploaded.DownloadURL != "" {
		emit("download", "Récupération du .torrent Hydracker…")
		data, derr := a.downloadHydrackerTorrent(uploaded.Torrent.ID)
		if derr != nil {
			return nil, fmt.Errorf("download torrent Hydracker : %w", derr)
		}
		hydTorPath = filepath.Join(filepath.Dir(mkvPath), releaseName+".hydracker.torrent")
		if err := os.WriteFile(hydTorPath, data, 0644); err != nil {
			return nil, fmt.Errorf("write torrent Hydracker : %w", err)
		}
		emit("download_done", fmt.Sprintf("Torrent Hydracker téléchargé (%d octets)", len(data)))
	}

	// 5. Upload sur seedbox
	emit("seedbox", "Upload sur seedbox…")
	sourceForSeedbox := hydTorPath
	if sourceForSeedbox == "" {
		sourceForSeedbox = torrentPath
	}
	seedPath, err := a.pushTorrent(a.workContext(), sourceForSeedbox, seedboxType, func(p seedbox.Progress) {
		wailsruntime.EventsEmit(a.ctx, "torrent:seedbox", p)
	})
	if err != nil {
		return nil, fmt.Errorf("seedbox: %w", err)
	}
	emit("seedbox_done", fmt.Sprintf("Seedbox OK : %s", seedPath))

	// Force re-check : le torrent vient d'être ajouté, on force la seedbox
	// (ruTorrent ou qBit selon config) à vérifier le hash du MKV déjà en place.
	if mi, err := metainfo.LoadFromFile(sourceForSeedbox); err == nil {
		hash := mi.HashInfoBytes().HexString()
		emit("recheck", "Force re-check seedbox…")
		time.Sleep(2 * time.Second)
		if err := a.recheckTorrent(hash, seedboxType); err != nil {
			emit("recheck_warn", "recheck échoué : "+err.Error())
		} else {
			emit("recheck_done", "Re-check OK")
		}
	}
	emit("done", fmt.Sprintf("Terminé : %s", seedPath))

	a.recordHistory(history.Entry{
		Type:        "torrent",
		TitleID:     titleID,
		TitleName:   a.titleName(titleID),
		Saison:      saison,
		Episode:     episode,
		Qualite:     qualite,
		QualiteName: a.qualiteName(qualite),
		HydrackerID: uploaded.Torrent.ID,
		Filename:    filepath.Base(mkvPath),
		Status:      "ok",
	})

	return &TorrentWorkflowResult{
		TorrentPath:      torrentPath,
		HydrackerID:      uploaded.Torrent.ID,
		HydrackerTorPath: hydTorPath,
		SeedboxPath:      seedPath,
	}, nil
}

// downloadHydrackerTorrent récupère le .torrent généré par Hydracker via l'API.
func (a *App) downloadHydrackerTorrent(torrentID int) ([]byte, error) {
	apiURL := fmt.Sprintf("%s/torrents/%d/download", a.client.BaseURL(), torrentID)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	if a.cfg.HydrackerToken != "" {
		req.Header.Set("Authorization", "Bearer "+a.cfg.HydrackerToken)
	}
	req.Header.Set("Accept", "application/x-bittorrent")
	req.Header.Set("User-Agent", "GoPostTools/3.0 (https://github.com/Gandalfleblanc/Go-Post-Tools)")
	c := &http.Client{Timeout: 60 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	if len(data) < 50 || data[0] != 'd' {
		return nil, fmt.Errorf("fichier reçu n'est pas un .torrent valide (Content-Type: %s)", resp.Header.Get("Content-Type"))
	}
	return data, nil
}

// --- Workflow NZB complet (ParPar → Nyuu → Hydracker) ---

type NzbWorkflowResult struct {
	NZBPath     string `json:"nzb_path"`
	HydrackerID int    `json:"hydracker_id"`
}

func (a *App) PostNzbWorkflow(titleID, qualite int, langues, subs []string, mkvPath, nfo string, saison, episode int) (*NzbWorkflowResult, error) {
	a.resetCancellation()
	// Validation config — retourne des messages explicites
	if a.cfg.UsenetHost == "" {
		return nil, fmt.Errorf("serveur Usenet non configuré (host vide) — renseignez les Settings")
	}
	if a.cfg.UsenetUser == "" {
		return nil, fmt.Errorf("identifiant Usenet manquant — renseignez les Settings")
	}
	if a.cfg.UsenetPort <= 0 {
		a.cfg.UsenetPort = 119
	}
	if a.cfg.UsenetGroup == "" {
		a.cfg.UsenetGroup = "alt.binaries.test"
	}
	mkvPath = strings.TrimSpace(mkvPath)
	if mkvPath == "" || mkvPath == "undefined" {
		return nil, fmt.Errorf("chemin du fichier MKV manquant — utilisez le bouton Parcourir")
	}

	ext := filepath.Ext(mkvPath)
	releaseName := strings.TrimSuffix(filepath.Base(mkvPath), ext)
	outputDir := filepath.Dir(mkvPath)

	// 1. ParPar
	wailsruntime.EventsEmit(a.ctx, "nzb:status", "Génération PAR2…")
	if err := parpar.Run(a.workContext(), a.cfg, mkvPath, func(p parpar.Progress) {
		wailsruntime.EventsEmit(a.ctx, "nzb:parpar", p)
	}); err != nil {
		return nil, fmt.Errorf("parpar: %w", err)
	}

	// 2. Collecter les fichiers (mkv + par2)
	par2Pattern := filepath.Join(outputDir, releaseName+".*.par2")
	par2Files, _ := filepath.Glob(par2Pattern)
	mainPar2 := filepath.Join(outputDir, releaseName+".par2")
	allFiles := []string{mkvPath}
	if _, err := os.Stat(mainPar2); err == nil {
		allFiles = append(allFiles, mainPar2)
	}
	allFiles = append(allFiles, par2Files...)

	// 3. Nyuu
	nzbPath := filepath.Join(outputDir, releaseName+".nzb")
	wailsruntime.EventsEmit(a.ctx, "nzb:status", "Post Usenet…")
	result, err := nyuu.Run(a.workContext(), a.cfg, allFiles, nzbPath, releaseName, func(p nyuu.Progress) {
		wailsruntime.EventsEmit(a.ctx, "nzb:nyuu", p)
	})
	if err != nil {
		return nil, fmt.Errorf("nyuu: %w", err)
	}

	// 4. Nettoyage par2
	os.Remove(mainPar2)
	for _, f := range par2Files {
		os.Remove(f)
	}

	if a.isCancelled() {
		return nil, fmt.Errorf("annulé par l'utilisateur")
	}
	// 5. Upload NZB sur Hydracker
	wailsruntime.EventsEmit(a.ctx, "nzb:status", "Upload NZB sur Hydracker…")
	uploaded, err := a.client.UploadNzb(titleID, qualite, langues, subs, result.NZBPath, nfo, saison, episode)
	if err != nil {
		return nil, fmt.Errorf("upload nzb: %w", err)
	}

	wailsruntime.EventsEmit(a.ctx, "nzb:status", "Terminé")
	wailsruntime.EventsEmit(a.ctx, "nzb:result", map[string]interface{}{
		"ok":      true,
		"message": fmt.Sprintf("NZB #%d posté avec succès", uploaded.Nzb.ID),
	})
	a.recordHistory(history.Entry{
		Type:        "nzb",
		TitleID:     titleID,
		TitleName:   a.titleName(titleID),
		Saison:      saison,
		Episode:     episode,
		Qualite:     qualite,
		QualiteName: a.qualiteName(qualite),
		HydrackerID: uploaded.Nzb.ID,
		Filename:    filepath.Base(mkvPath),
		Status:      "ok",
	})
	return &NzbWorkflowResult{
		NZBPath:     result.NZBPath,
		HydrackerID: uploaded.Nzb.ID,
	}, nil
}

// --- Workflow DDL complet (1Fichier + Send.now → Hydracker) ---

type DDLWorkflowResult struct {
	Links       []string `json:"links"`
	HydrackerID int      `json:"hydracker_id"`
}

func (a *App) PostDDLWorkflow(titleID, qualite int, langues, subs []string, mkvPath, nfo string, use1Fichier, useSendCm bool, saison, episode int) (*DDLWorkflowResult, error) {
	a.resetCancellation()
	if mkvPath == "" {
		return nil, fmt.Errorf("chemin MKV manquant")
	}
	if !use1Fichier && !useSendCm {
		return nil, fmt.Errorf("aucun host sélectionné (1Fichier et Send.now décochés)")
	}
	if use1Fichier && a.cfg.OneFichierApiKey == "" && !useSendCm {
		return nil, fmt.Errorf("clé 1Fichier manquante — renseignez les Settings")
	}
	if useSendCm && a.cfg.SendCmApiKey == "" && !use1Fichier {
		return nil, fmt.Errorf("clé Send.now manquante — renseignez les Settings")
	}

	filename := filepath.Base(mkvPath)
	logEvent := func(msg string) {
		wailsruntime.EventsEmit(a.ctx, "ddl:log", msg)
	}
	emitProgress := func(host string, p uploader.UploadProgress) {
		wailsruntime.EventsEmit(a.ctx, "ddl:progress", map[string]interface{}{
			"host":     host,
			"filename": filename,
			"percent":  p.Percent,
			"speed":    fmt.Sprintf("%.1f MB/s", p.SpeedMB),
		})
	}
	emitDone := func(host string, err error) {
		if err != nil {
			wailsruntime.EventsEmit(a.ctx, "ddl:done", map[string]interface{}{"host": host, "error": err.Error()})
		} else {
			wailsruntime.EventsEmit(a.ctx, "ddl:done", map[string]interface{}{"host": host, "error": ""})
		}
	}

	type uploadResult struct {
		url     string
		err     error
		skipped bool // host annulé individuellement par l'utilisateur
	}

	// registerHostCancel stocke la CancelFunc du host pour que CancelDDLHost puisse l'appeler.
	registerHostCancel := func(host string, cancel context.CancelFunc) {
		a.hostMu.Lock()
		a.hostCancels[host] = cancel
		a.hostMu.Unlock()
	}
	unregisterHostCancel := func(host string) {
		a.hostMu.Lock()
		delete(a.hostCancels, host)
		a.hostMu.Unlock()
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	results := map[string]uploadResult{}

	if use1Fichier && a.cfg.OneFichierApiKey != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hostCtx, hostCancel := context.WithCancel(a.workContext())
			registerHostCancel("1Fichier", hostCancel)
			defer unregisterHostCancel("1Fichier")
			defer hostCancel()
			logEvent("1Fichier : connexion au serveur…")
			res, err := uploader.UploadOneFichier(hostCtx, a.cfg.OneFichierApiKey, mkvPath, func(p uploader.UploadProgress) {
				emitProgress("1Fichier", p)
			})
			emitDone("1Fichier", err)
			mu.Lock()
			switch {
			case err == nil:
				results["1Fichier"] = uploadResult{url: res.URL}
				logEvent("1Fichier : upload terminé ✓")
			case hostCtx.Err() != nil && a.workContext().Err() == nil:
				results["1Fichier"] = uploadResult{skipped: true}
				logEvent("1Fichier : skippé par l'utilisateur")
			default:
				results["1Fichier"] = uploadResult{err: err}
				logEvent(fmt.Sprintf("1Fichier : erreur — %s", err.Error()))
			}
			mu.Unlock()
		}()
	}

	if useSendCm && a.cfg.SendCmApiKey != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hostCtx, hostCancel := context.WithCancel(a.workContext())
			registerHostCancel("Send.now", hostCancel)
			defer unregisterHostCancel("Send.now")
			defer hostCancel()
			logEvent("Send.now : connexion au serveur…")
			res, err := uploader.UploadSendCm(hostCtx, a.cfg.SendCmApiKey, mkvPath, func(p uploader.UploadProgress) {
				emitProgress("Send.now", p)
			})
			emitDone("Send.now", err)
			mu.Lock()
			switch {
			case err == nil:
				results["Send.now"] = uploadResult{url: res.URL}
				logEvent("Send.now : upload terminé ✓")
			case hostCtx.Err() != nil && a.workContext().Err() == nil:
				results["Send.now"] = uploadResult{skipped: true}
				logEvent("Send.now : skippé par l'utilisateur")
			default:
				results["Send.now"] = uploadResult{err: err}
				logEvent(fmt.Sprintf("Send.now : erreur — %s", err.Error()))
			}
			mu.Unlock()
		}()
	}

	wg.Wait()

	// Vérifier les erreurs (on ignore les skipped — ils ne sont pas considérés comme erreur)
	anyOK := false
	for host, r := range results {
		if r.skipped {
			continue
		}
		if r.err != nil {
			// Si TOUS les hosts sont en erreur ou skipped, on échoue. Sinon on continue.
			return nil, fmt.Errorf("%s: %w", strings.ToLower(host), r.err)
		}
		anyOK = true
	}
	if !anyOK {
		return nil, fmt.Errorf("tous les hosts ont été skippés ou ont échoué")
	}

	// Post tous les liens sur Hydracker (skipper les hosts skippés/sans URL)
	var links []string
	var lastHydrackerID int
	for host, r := range results {
		if r.skipped || r.url == "" {
			continue
		}
		logEvent(fmt.Sprintf("%s : post du lien sur Hydracker…", host))
		wailsruntime.EventsEmit(a.ctx, "ddl:posting", map[string]interface{}{"host": host, "posting": true})
		uploaded, err := a.client.UploadLien(titleID, qualite, langues, subs, r.url, nfo, saison, episode)
		if err != nil {
			wailsruntime.EventsEmit(a.ctx, "ddl:posting", map[string]interface{}{"host": host, "posting": false})
			return nil, fmt.Errorf("hydracker %s: %w", strings.ToLower(host), err)
		}
		wailsruntime.EventsEmit(a.ctx, "ddl:posting", map[string]interface{}{"host": host, "posting": false, "posted": true, "id": uploaded.Lien().ID})
		links = append(links, r.url)
		lastHydrackerID = uploaded.Lien().ID
		epSuffix := ""
		if saison > 0 || episode > 0 {
			epSuffix = fmt.Sprintf(" S%02dE%02d", saison, episode)
		}
		if uploaded.Lien().ID > 0 {
			logEvent(fmt.Sprintf("%s : lien #%d ajouté ✓ — %s%s → %s", host, uploaded.Lien().ID, filename, epSuffix, r.url))
		} else {
			logEvent(fmt.Sprintf("%s : lien ajouté sur Hydracker ✓", host))
		}
	}

	logEvent("DDL terminé ✓")
	a.recordHistory(history.Entry{
		Type:        "ddl",
		TitleID:     titleID,
		TitleName:   a.titleName(titleID),
		Saison:      saison,
		Episode:     episode,
		Qualite:     qualite,
		QualiteName: a.qualiteName(qualite),
		HydrackerID: lastHydrackerID,
		Filename:    filename,
		Links:       strings.Join(links, "\n"),
		Status:      "ok",
	})
	return &DDLWorkflowResult{
		Links:       links,
		HydrackerID: lastHydrackerID,
	}, nil
}
