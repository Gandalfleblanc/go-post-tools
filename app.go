package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"os"
	"path/filepath"
	"strings"

	"sync"

	"go-post-tools/api"
	"go-post-tools/internal/config"
	"go-post-tools/internal/ftpup"
	"go-post-tools/internal/history"
	"go-post-tools/internal/lihdl"
	"go-post-tools/internal/nyuu"
	"go-post-tools/internal/rutorrent"
	"go-post-tools/internal/mediasearch"

	"github.com/anacrolix/torrent/metainfo"
	"go-post-tools/internal/parpar"
	"go-post-tools/internal/parser"
	"go-post-tools/internal/seedbox"
	"go-post-tools/internal/tester"
	"go-post-tools/internal/tmdb"
	"go-post-tools/internal/torrent"
	"go-post-tools/internal/uploader"
	"go-post-tools/internal/watcher"

	"github.com/gen2brain/beeep"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const Version = "1.2.16"

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

	// Ouvrir le dossier Téléchargements au bon fichier
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

func NewApp() *App {
	cfg := config.Load()
	hist, _ := history.Open()
	return &App{
		client:      api.NewClient(cfg.HydrackerToken, cfg.HydrackerBaseURL),
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
}

// --- Config ---

func (a *App) GetConfig() *config.Config {
	return a.cfg
}

func (a *App) SaveConfig(cfg config.Config) error {
	a.cfg = &cfg
	a.client.SetToken(cfg.HydrackerToken)
	a.client.SetBaseURL(cfg.HydrackerBaseURL)
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

func (a *App) TestUsenet(host string, port int) tester.Result {
	return tester.TestUsenet(host, port)
}

// --- TMDB ---

func (a *App) TMDBSearch(query string) ([]tmdb.Movie, error) {
	return tmdb.NewClient(a.cfg.TMDBApiKey).Search(query)
}

func (a *App) TMDBGetByID(id int, mediaType string) (*tmdb.Movie, error) {
	return tmdb.NewClient(a.cfg.TMDBApiKey).GetByID(id, mediaType)
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
	return a.client.GetQualities()
}

func (a *App) GetMetaLangs() ([]api.Lang, error) {
	return a.client.GetLangs()
}

func (a *App) GetMetaSubs() ([]api.Lang, error) {
	return a.client.GetSubs()
}

// --- Hydracker search ---

func (a *App) HydrackerSearch(query string) ([]api.PartialTitle, error) {
	result, err := a.client.Search(query, 10)
	if err != nil {
		return nil, err
	}
	return result.Titles, nil
}

func (a *App) HydrackerGetByTmdbID(tmdbID int) (*api.PartialTitle, error) {
	return a.client.GetTitleByTmdbID(tmdbID)
}

func (a *App) HydrackerGetByID(id int) (*api.PartialTitle, error) {
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

func (a *App) FicheGetContent(titleID int) (*FicheContent, error) {
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
	if _, err := seedbox.Upload(a.workContext(), a.cfg.SeedboxURL, a.cfg.SeedboxUser, a.cfg.SeedboxPassword, a.cfg.SeedboxLabel, torrentPath, nil); err != nil {
		emit("error", "seedbox: "+err.Error())
		return err
	}

	// 3. Force re-check (le torrent vient d'être ajouté, il va check le MKV uploadé)
	mi, err := metainfo.LoadFromFile(torrentPath)
	if err == nil {
		hash := mi.HashInfoBytes().HexString()
		emit("recheck", "Re-check…")
		// petite pause pour laisser rtorrent enregistrer le torrent
		time.Sleep(2 * time.Second)
		_ = rutorrent.Recheck(a.cfg.SeedboxURL, a.cfg.SeedboxUser, a.cfg.SeedboxPassword, hash)
	}
	emit("done", "Terminé")
	return nil
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

	emit("recheck", "Re-check ruTorrent…")
	if err := rutorrent.Recheck(a.cfg.SeedboxURL, a.cfg.SeedboxUser, a.cfg.SeedboxPassword, hash); err != nil {
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

func (a *App) PostTorrentWorkflow(titleID, qualite int, langues, subs []string, mkvPath, nfo string, saison, episode int) (*TorrentWorkflowResult, error) {
	a.resetCancellation()
	if strings.TrimSpace(mkvPath) == "" {
		return nil, fmt.Errorf("chemin MKV manquant")
	}
	if a.cfg.FTPHost == "" || a.cfg.FTPUser == "" {
		return nil, fmt.Errorf("FTP non configuré — renseignez les Settings")
	}
	if a.cfg.SeedboxURL == "" {
		return nil, fmt.Errorf("seedbox non configurée — renseignez les Settings")
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

	// 1. Upload MKV sur FTP
	emit("ftp", "Upload FTP…")
	remoteName, err := ftpup.Upload(a.workContext(), a.cfg.FTPHost, a.cfg.FTPPort, a.cfg.FTPUser, a.cfg.FTPPassword, a.cfg.FTPPath, mkvPath, func(p ftpup.Progress) {
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
	seedPath, err := seedbox.Upload(a.workContext(), a.cfg.SeedboxURL, a.cfg.SeedboxUser, a.cfg.SeedboxPassword, a.cfg.SeedboxLabel, sourceForSeedbox, func(p seedbox.Progress) {
		wailsruntime.EventsEmit(a.ctx, "torrent:seedbox", p)
	})
	if err != nil {
		return nil, fmt.Errorf("seedbox: %w", err)
	}
	emit("done", fmt.Sprintf("Seedbox OK : %s", seedPath))

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
