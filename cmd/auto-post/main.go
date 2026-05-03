// auto-post : RSS NZB → SABnzbd → MKV → (futur : post Hydracker).
//
// Successeur de cmd/autopush/. Reprend le polling RSS + filtres + anti-doublons,
// mais au lieu de juste envoyer une notif Telegram, soumet le .nzb à SABnzbd
// puis suit le téléchargement jusqu'à récupération du MKV.
//
// Étape suivante (chunk #5) : pousser le MKV vers Hydracker via internal/uploader.
package main

import (
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"go-post-tools/api"
	"go-post-tools/internal/tmdb"

	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)

// ---------- Config ----------

type Config struct {
	RSS struct {
		URL          string        `yaml:"url"`
		PollInterval time.Duration `yaml:"poll_interval"`
	} `yaml:"rss"`
	Filters struct {
		AllowedCategories         []string `yaml:"allowed_categories"`
		AllowedTeams              []string `yaml:"allowed_teams"`
		BlockedTeams              []string `yaml:"blocked_teams"`
		ExcludeKeywords           []string `yaml:"exclude_keywords"`
		AllowedHydrackerQualities []int    `yaml:"allowed_hydracker_qualities"`
	} `yaml:"filters"`
	Telegram struct {
		BotToken string `yaml:"bot_token"`
		ChatID   string `yaml:"chat_id"`
	} `yaml:"telegram"`
	Storage struct {
		DBPath string `yaml:"db_path"`
	} `yaml:"storage"`
	SABnzbd struct {
		URL          string        `yaml:"url"`
		APIKey       string        `yaml:"api_key"`
		Category     string        `yaml:"category"`
		PollInterval time.Duration `yaml:"poll_interval"`
	} `yaml:"sabnzbd"`
	TMDB struct {
		ProxyURL     string        `yaml:"proxy_url"` // vide = défaut tmdb.uklm.xyz
		PollInterval time.Duration `yaml:"poll_interval"`
	} `yaml:"tmdb"`
	Hydracker struct {
		BaseURL      string        `yaml:"base_url"`
		Token        string        `yaml:"token"`
		PollInterval time.Duration `yaml:"poll_interval"`
	} `yaml:"hydracker"`
	OneFichier struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"one_fichier"`
	SendCm struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"sendcm"`
	FTP struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		Path     string `yaml:"path"`
	} `yaml:"ftp"`
	Seedbox struct {
		URL      string `yaml:"url"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
	} `yaml:"seedbox"`
	Torrent struct {
		TrackerURL string `yaml:"tracker_url"`
		PieceSize  int64  `yaml:"piece_size"`
	} `yaml:"torrent"`
	IRC struct {
		Enabled            bool   `yaml:"enabled"`
		Host               string `yaml:"host"`
		Port               int    `yaml:"port"`
		Nick               string `yaml:"nick"`
		InviteBot          string `yaml:"invite_bot"`    // ex: "UNFR_BOT"
		AnnounceBot        string `yaml:"announce_bot"`  // qui poste les annonces (souvent même)
		UnfrKey            string `yaml:"unfr_key"`      // clé d'auth user pour /msg invite
		UnfrGetURL         string `yaml:"unfr_get_url"`  // base URL get NZB (ex: https://unfr.pw/get.php)
		InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
		LogRaw             bool   `yaml:"log_raw"` // dump toutes les lignes IRC en log (debug)
	} `yaml:"irc"`
	Ntfy struct {
		URL            string `yaml:"url"`             // ex: http://100.72.205.55:8000
		Topic          string `yaml:"topic"`           // ex: auto-post
		User           string `yaml:"user"`            // basic auth
		Password       string `yaml:"password"`        // basic auth
		WebhookListen  string `yaml:"webhook_listen"`  // ex: ":8081"
		WebhookBaseURL string `yaml:"webhook_base_url"` // URL publique du webhook (Tailnet)
		WebhookSecret  string `yaml:"webhook_secret"`  // shared secret X-Auto-Post-Token
	} `yaml:"ntfy"`
	Discord struct {
		Token     string `yaml:"token"`      // bot token
		ChannelID string `yaml:"channel_id"` // channel où poster
	} `yaml:"discord"`
	AutoConfirm struct {
		Enabled      bool     `yaml:"enabled"`       // auto-confirm sans clic user
		MinScore     float64  `yaml:"min_score"`     // score TMDB minimum (def 0.90)
		TrustedTeams []string `yaml:"trusted_teams"` // teams 100% fiables (vide = toutes whitelistées)
		ApplyCatchup bool     `yaml:"apply_catchup"` // applique aussi sur les catch-up/bulk
	} `yaml:"auto_confirm"`
	Monitoring struct {
		Enabled              bool `yaml:"enabled"`
		AlertNoPostDays      int  `yaml:"alert_no_post_days"`      // alerte si 0 post depuis N jours (def 3)
		AlertDiskPercent     int  `yaml:"alert_disk_percent"`      // alerte si disque >% utilisé (def 85)
		AlertHydrackerDownMin int `yaml:"alert_hydracker_down_min"` // alerte si Hydracker KO depuis N min (def 60)
	} `yaml:"monitoring"`
}

func loadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.RSS.PollInterval == 0 {
		c.RSS.PollInterval = 5 * time.Minute
	}
	if c.SABnzbd.PollInterval == 0 {
		c.SABnzbd.PollInterval = 30 * time.Second
	}
	if c.TMDB.PollInterval == 0 {
		c.TMDB.PollInterval = 30 * time.Second
	}
	if c.Hydracker.PollInterval == 0 {
		c.Hydracker.PollInterval = 60 * time.Second
	}
	if c.Torrent.PieceSize == 0 {
		c.Torrent.PieceSize = 8 * 1024 * 1024
	}
	if c.Storage.DBPath == "" {
		c.Storage.DBPath = "auto-post.db"
	}
	return &c, nil
}

// ---------- RSS ----------

type RSSItem struct {
	Title    string `xml:"title"`
	Link     string `xml:"link"`
	PubDate  string `xml:"pubDate"`
	Category string `xml:"category"`
	GUID     string `xml:"guid"`
}

type RSSFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []RSSItem `xml:"item"`
	} `xml:"channel"`
}

func fetchRSS(rssURL string) (*RSSFeed, error) {
	req, _ := http.NewRequest("GET", rssURL, nil)
	req.Header.Set("User-Agent", "go-post-tools-auto-post/1.0")
	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	var feed RSSFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("parse RSS: %w", err)
	}
	return &feed, nil
}

// ---------- Filters ----------

var teamRE = regexp.MustCompile(`-([A-Za-z0-9]+)$`)

func extractTeam(title string) string {
	m := teamRE.FindStringSubmatch(title)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func contains(slice []string, s string) bool {
	s = strings.ToLower(s)
	for _, v := range slice {
		if strings.ToLower(v) == s {
			return true
		}
	}
	return false
}

func containsAny(haystack string, needles []string) bool {
	hl := strings.ToLower(haystack)
	for _, n := range needles {
		if n != "" && strings.Contains(hl, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

func passesFilters(cfg *Config, item RSSItem, team string) (bool, string) {
	if len(cfg.Filters.AllowedCategories) > 0 && !contains(cfg.Filters.AllowedCategories, item.Category) {
		return false, "cat exclue: " + item.Category
	}
	if len(cfg.Filters.BlockedTeams) > 0 && contains(cfg.Filters.BlockedTeams, team) {
		return false, "team blacklist: " + team
	}
	if len(cfg.Filters.AllowedTeams) > 0 && !contains(cfg.Filters.AllowedTeams, team) {
		return false, "team hors whitelist: " + team
	}
	if containsAny(item.Title, cfg.Filters.ExcludeKeywords) {
		return false, "keyword exclu"
	}
	return true, ""
}

// ---------- DB ----------

func openDB(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS seen_items (
			guid TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			category TEXT,
			team TEXT,
			link TEXT,
			seen_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_seen_at ON seen_items(seen_at);

		CREATE TABLE IF NOT EXISTS jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			guid TEXT NOT NULL,
			title TEXT NOT NULL,
			category TEXT,
			team TEXT,
			nzb_url TEXT NOT NULL,
			nzo_id TEXT,
			status TEXT NOT NULL,
			mkv_path TEXT,
			error TEXT,
			submitted_at INTEGER NOT NULL,
			completed_at INTEGER,
			notified_at INTEGER,
			tmdb_id INTEGER,
			tmdb_title TEXT,
			tmdb_year TEXT,
			tmdb_score REAL,
			tmdb_status TEXT,
			tmdb_poster TEXT,
			tmdb_checked_at INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
		CREATE INDEX IF NOT EXISTS idx_jobs_nzo ON jobs(nzo_id);
	`)
	if err != nil {
		return nil, err
	}
	// Migration : ajoute les colonnes tmdb_*/hydracker_* si la DB existait avant
	for _, alter := range []string{
		"ALTER TABLE jobs ADD COLUMN tmdb_id INTEGER",
		"ALTER TABLE jobs ADD COLUMN tmdb_title TEXT",
		"ALTER TABLE jobs ADD COLUMN tmdb_year TEXT",
		"ALTER TABLE jobs ADD COLUMN tmdb_score REAL",
		"ALTER TABLE jobs ADD COLUMN tmdb_status TEXT",
		"ALTER TABLE jobs ADD COLUMN tmdb_poster TEXT",
		"ALTER TABLE jobs ADD COLUMN tmdb_checked_at INTEGER",
		"ALTER TABLE jobs ADD COLUMN tmdb_alts_json TEXT",
		"ALTER TABLE jobs ADD COLUMN telegram_message_id INTEGER",
		"ALTER TABLE jobs ADD COLUMN hydracker_status TEXT",
		"ALTER TABLE jobs ADD COLUMN hydracker_title_id INTEGER",
		"ALTER TABLE jobs ADD COLUMN hydracker_torrent_id INTEGER",
		"ALTER TABLE jobs ADD COLUMN hydracker_ddl_urls_json TEXT",
		"ALTER TABLE jobs ADD COLUMN hydracker_error TEXT",
		"ALTER TABLE jobs ADD COLUMN hydracker_processed_at INTEGER",
		"ALTER TABLE jobs ADD COLUMN cleanup_done_at INTEGER",
		"ALTER TABLE jobs ADD COLUMN via_bulk_import INTEGER DEFAULT 0",
		"ALTER TABLE jobs ADD COLUMN linked_to_job_id INTEGER",
		"ALTER TABLE jobs ADD COLUMN hydracker_nzb_id INTEGER",
	} {
		_, _ = db.Exec(alter) // ignore "duplicate column" errors
	}
	// État global du bot (last update_id pour getUpdates)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS bot_state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`)
	if err != nil {
		return nil, err
	}
	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_jobs_tmdb_status ON jobs(tmdb_status)")
	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_jobs_hydracker_status ON jobs(hydracker_status)")
	return db, nil
}

// ---------- Telegram ----------

func sendTelegram(botToken, chatID, text string, silent bool) error {
	api := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	form.Set("parse_mode", "HTML")
	form.Set("disable_web_page_preview", "true")
	if silent {
		form.Set("disable_notification", "true")
	}
	resp, err := http.PostForm(api, form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("<", "&lt;", ">", "&gt;", "&", "&amp;")
	return r.Replace(s)
}

// sendTelegramPhoto envoie une photo (URL) avec caption HTML.
// Si photoURL est vide, fallback sur sendMessage texte.
func sendTelegramPhoto(botToken, chatID, photoURL, caption string, silent bool) error {
	if photoURL == "" {
		return sendTelegram(botToken, chatID, caption, silent)
	}
	api := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", botToken)
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("photo", photoURL)
	form.Set("caption", caption)
	form.Set("parse_mode", "HTML")
	if silent {
		form.Set("disable_notification", "true")
	}
	resp, err := http.PostForm(api, form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		// Fallback : si Telegram refuse la photo (URL morte), envoie en texte
		_ = sendTelegram(botToken, chatID, caption, silent)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func formatNotifSubmitted(item RSSItem, team string) string {
	emoji := "📦"
	if strings.HasPrefix(item.Category, "TV-") {
		emoji = "📺"
	} else if strings.HasPrefix(item.Category, "MOVIE-") {
		emoji = "🎬"
	}
	teamLine := ""
	if team != "" {
		teamLine = fmt.Sprintf("<b>👥 Team:</b> %s\n", htmlEscape(team))
	}
	return fmt.Sprintf(
		"%s <b>NZB en téléchargement</b>\n\n<b>%s</b>\n\n<b>📁 Catégorie:</b> %s\n%s",
		emoji, htmlEscape(item.Title), htmlEscape(item.Category), teamLine,
	)
}

func formatNotifCompleted(title, mkvPath string) string {
	return fmt.Sprintf(
		"📥 <b>MKV récupéré</b>\n\n<b>%s</b>\n\n<code>%s</code>",
		htmlEscape(title), htmlEscape(mkvPath),
	)
}

func formatNotifFailed(title, errMsg string) string {
	return fmt.Sprintf(
		"❌ <b>Téléchargement échoué</b>\n\n<b>%s</b>\n\n%s",
		htmlEscape(title), htmlEscape(errMsg),
	)
}

// ---------- SABnzbd API ----------

type sabAddResp struct {
	Status bool     `json:"status"`
	NzoIds []string `json:"nzo_ids"`
	Error  string   `json:"error"`
}

type sabQueueResp struct {
	Queue struct {
		Slots []struct {
			NzoID  string `json:"nzo_id"`
			Status string `json:"status"`
			Filename string `json:"filename"`
			Percentage string `json:"percentage"`
		} `json:"slots"`
	} `json:"queue"`
}

type sabHistoryResp struct {
	History struct {
		Slots []struct {
			NzoID    string `json:"nzo_id"`
			Status   string `json:"status"`
			Storage  string `json:"storage"`
			Name     string `json:"name"`
			FailMessage string `json:"fail_message"`
		} `json:"slots"`
	} `json:"history"`
}

func sabAPI(cfg *Config, params url.Values) ([]byte, error) {
	params.Set("apikey", cfg.SABnzbd.APIKey)
	params.Set("output", "json")
	apiURL := strings.TrimRight(cfg.SABnzbd.URL, "/") + "/api?" + params.Encode()
	req, _ := http.NewRequest("GET", apiURL, nil)
	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("SAB HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func sabAddURL(cfg *Config, nzbURL, name string) (string, error) {
	params := url.Values{}
	params.Set("mode", "addurl")
	params.Set("name", nzbURL)
	if name != "" {
		params.Set("nzbname", name)
	}
	if cfg.SABnzbd.Category != "" {
		params.Set("cat", cfg.SABnzbd.Category)
	}
	body, err := sabAPI(cfg, params)
	if err != nil {
		return "", err
	}
	var r sabAddResp
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("parse addurl: %w (body=%s)", err, string(body))
	}
	if !r.Status {
		return "", fmt.Errorf("SAB addurl rejected: %s", r.Error)
	}
	if len(r.NzoIds) == 0 {
		return "", fmt.Errorf("SAB addurl: no nzo_id returned")
	}
	return r.NzoIds[0], nil
}

func sabQueueStatus(cfg *Config, nzoID string) (status string, found bool, err error) {
	params := url.Values{}
	params.Set("mode", "queue")
	body, err := sabAPI(cfg, params)
	if err != nil {
		return "", false, err
	}
	var r sabQueueResp
	if err := json.Unmarshal(body, &r); err != nil {
		return "", false, err
	}
	for _, s := range r.Queue.Slots {
		if s.NzoID == nzoID {
			return s.Status, true, nil
		}
	}
	return "", false, nil
}

func sabHistoryStatus(cfg *Config, nzoID string) (status, storage, failMsg string, found bool, err error) {
	params := url.Values{}
	params.Set("mode", "history")
	params.Set("limit", "100")
	body, err := sabAPI(cfg, params)
	if err != nil {
		return "", "", "", false, err
	}
	var r sabHistoryResp
	if err := json.Unmarshal(body, &r); err != nil {
		return "", "", "", false, err
	}
	for _, s := range r.History.Slots {
		if s.NzoID == nzoID {
			return s.Status, s.Storage, s.FailMessage, true, nil
		}
	}
	return "", "", "", false, nil
}

// findMKV : si storage est déjà un .mkv → le retourne. Si c'est un dossier,
// walke pour trouver le plus gros .mkv (ignore les sample.mkv).
func findMKV(storage string) (string, error) {
	info, err := os.Stat(storage)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		if strings.EqualFold(filepath.Ext(storage), ".mkv") {
			return storage, nil
		}
		return "", fmt.Errorf("storage is not an MKV file: %s", storage)
	}
	var best string
	var bestSize int64
	err = filepath.Walk(storage, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(p), ".mkv") {
			return nil
		}
		base := strings.ToLower(filepath.Base(p))
		if strings.Contains(base, "sample") {
			return nil
		}
		if info.Size() > bestSize {
			bestSize = info.Size()
			best = p
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if best == "" {
		return "", fmt.Errorf("no MKV found in %s", storage)
	}
	return best, nil
}

// ---------- Pipeline ----------

// processFeed traite tous les nouveaux items du flux. Pour chaque item qui passe
// les filtres : ajoute en seen_items, soumet à SABnzbd, crée un job en DB.
func processFeed(cfg *Config, db *sql.DB) (int, int, error) {
	if isPaused() {
		return 0, 0, nil
	}
	feed, err := fetchRSS(cfg.RSS.URL)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch RSS: %w", err)
	}
	now := time.Now().Unix()
	newCount := 0
	submitCount := 0

	for _, item := range feed.Channel.Items {
		if item.GUID == "" {
			continue
		}
		var existing string
		err := db.QueryRow("SELECT guid FROM seen_items WHERE guid = ?", item.GUID).Scan(&existing)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			log.Printf("[auto-post] DB query err: %v", err)
			continue
		}
		newCount++
		team := extractTeam(item.Title)
		ok, reason := passesFilters(cfg, item, team)

		_, err = db.Exec(
			"INSERT INTO seen_items (guid, title, category, team, link, seen_at) VALUES (?, ?, ?, ?, ?, ?)",
			item.GUID, item.Title, item.Category, team, item.Link, now,
		)
		if err != nil {
			log.Printf("[auto-post] DB insert err: %v", err)
			continue
		}

		if !ok {
			log.Printf("[auto-post] SKIP %s (%s)", item.Title, reason)
			continue
		}

		// Soumet à SABnzbd
		log.Printf("[auto-post] SUBMIT %s [team=%s cat=%s]", item.Title, team, item.Category)
		nzoID, err := sabAddURL(cfg, item.Link, item.Title)
		if err != nil {
			log.Printf("[auto-post] SAB submit err: %v", err)
			_, _ = db.Exec(
				"INSERT INTO jobs (guid, title, category, team, nzb_url, status, error, submitted_at) VALUES (?, ?, ?, ?, ?, 'submit_failed', ?, ?)",
				item.GUID, item.Title, item.Category, team, item.Link, err.Error(), now,
			)
			notifyFailed(cfg, item.Title, "SAB submit error: "+err.Error())
			continue
		}

		_, err = db.Exec(
			"INSERT INTO jobs (guid, title, category, team, nzb_url, nzo_id, status, submitted_at) VALUES (?, ?, ?, ?, ?, ?, 'downloading', ?)",
			item.GUID, item.Title, item.Category, team, item.Link, nzoID, now,
		)
		if err != nil {
			log.Printf("[auto-post] DB job insert err: %v", err)
		}
		submitCount++
		notifySubmitted(cfg, item, team)
		time.Sleep(150 * time.Millisecond)
	}
	return newCount, submitCount, nil
}

// pollJobs : pour chaque job actif (downloading), check SAB queue puis history.
// Si l'item a quitté la queue et est en history avec status Completed → trouve
// le MKV, met à jour status="ready", envoie notif.
func pollJobs(cfg *Config, db *sql.DB) error {
	if isPaused() {
		return nil
	}
	rows, err := db.Query("SELECT id, guid, title, nzo_id FROM jobs WHERE status = 'downloading'")
	if err != nil {
		return err
	}
	defer rows.Close()

	type pendingJob struct {
		ID    int64
		GUID  string
		Title string
		NzoID string
	}
	var pending []pendingJob
	for rows.Next() {
		var j pendingJob
		if err := rows.Scan(&j.ID, &j.GUID, &j.Title, &j.NzoID); err != nil {
			return err
		}
		pending = append(pending, j)
	}
	rows.Close()

	for _, j := range pending {
		// 1. encore dans la queue ?
		_, inQueue, err := sabQueueStatus(cfg, j.NzoID)
		if err != nil {
			log.Printf("[auto-post] SAB queue err for %s: %v", j.NzoID, err)
			continue
		}
		if inQueue {
			continue // toujours en cours
		}

		// 2. dans l'history
		status, storage, failMsg, found, err := sabHistoryStatus(cfg, j.NzoID)
		if err != nil {
			log.Printf("[auto-post] SAB history err for %s: %v", j.NzoID, err)
			continue
		}
		if !found {
			// Pas en queue ni history : retiré ? on retry plus tard
			continue
		}

		now := time.Now().Unix()
		switch strings.ToLower(status) {
		case "completed":
			mkvPath, err := findMKV(storage)
			if err != nil {
				log.Printf("[auto-post] findMKV err for %s: %v", j.Title, err)
				_, _ = db.Exec(
					"UPDATE jobs SET status='no_mkv', error=?, completed_at=?, notified_at=? WHERE id=?",
					err.Error(), now, now, j.ID,
				)
				notifyFailed(cfg, j.Title, "Pas de MKV trouvé dans "+storage)
				continue
			}
			_, err = db.Exec(
				"UPDATE jobs SET status='ready', mkv_path=?, completed_at=?, notified_at=? WHERE id=?",
				mkvPath, now, now, j.ID,
			)
			if err != nil {
				log.Printf("[auto-post] DB update err: %v", err)
			}
			log.Printf("[auto-post] READY %s → %s", j.Title, mkvPath)
			notifyCompleted(cfg, j.Title, mkvPath)
		case "failed":
			_, _ = db.Exec(
				"UPDATE jobs SET status='failed', error=?, completed_at=?, notified_at=? WHERE id=?",
				failMsg, now, now, j.ID,
			)
			log.Printf("[auto-post] FAILED %s : %s", j.Title, failMsg)
			notifyFailed(cfg, j.Title, failMsg)
		default:
			// status inconnu (ex: "Queued" if just moved) — on attend
			log.Printf("[auto-post] %s status=%s, waiting", j.Title, status)
		}
	}
	return nil
}

// pollAwaitingDL : pour chaque job avec status='awaiting_dl' ET tmdb_status='confirmed',
// soumet le NZB à SABnzbd et passe en status='downloading'. C'est l'étape
// déclenchée APRÈS confirmation user (mode review-first).
func pollAwaitingDL(cfg *Config, db *sql.DB) error {
	if isPaused() {
		return nil
	}
	rows, err := db.Query(`SELECT id, title, nzb_url FROM jobs
		WHERE status='awaiting_dl' AND tmdb_status='confirmed'
		ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type pendingDL struct {
		ID     int64
		Title  string
		NzbURL string
	}
	var pending []pendingDL
	for rows.Next() {
		var p pendingDL
		if err := rows.Scan(&p.ID, &p.Title, &p.NzbURL); err != nil {
			return err
		}
		pending = append(pending, p)
	}
	rows.Close()

	for _, p := range pending {
		log.Printf("[awaiting-dl] SUBMIT %s", p.Title)
		nzoID, err := sabAddURL(cfg, p.NzbURL, p.Title)
		now := time.Now().Unix()
		if err != nil {
			log.Printf("[awaiting-dl] SAB submit err: %v", err)
			_, _ = db.Exec(
				"UPDATE jobs SET status='submit_failed', error=?, completed_at=? WHERE id=?",
				err.Error(), now, p.ID,
			)
			continue
		}
		_, _ = db.Exec(
			"UPDATE jobs SET status='downloading', nzo_id=? WHERE id=?",
			nzoID, p.ID,
		)
		time.Sleep(500 * time.Millisecond) // throttle SAB
	}
	return nil
}

// pollTMDB : pour chaque job avec status='ready' et tmdb_status NULL,
// fait un lookup TMDB (parse filename → score → notif).
// Si via_bulk_import=1, on force tmdb_status='pending' même sur high_confidence
// (l'user veut décider lui-même pour chaque bulk import, pas d'auto-post).
func pollTMDB(cfg *Config, db *sql.DB, tmdbClient *tmdb.Client) error {
	if isPaused() {
		return nil
	}
	rows, err := db.Query(`SELECT id, title, COALESCE(mkv_path,''), COALESCE(via_bulk_import,0) FROM jobs
		WHERE status = 'ready' AND tmdb_status IS NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type pendingTMDB struct {
		ID         int64
		Title      string
		MkvPath    string
		BulkImport int
	}
	var pending []pendingTMDB
	for rows.Next() {
		var p pendingTMDB
		if err := rows.Scan(&p.ID, &p.Title, &p.MkvPath, &p.BulkImport); err != nil {
			return err
		}
		pending = append(pending, p)
	}
	rows.Close()

	for _, p := range pending {
		log.Printf("[auto-post] TMDB lookup: %s", p.Title)
		res := lookupTMDB(tmdbClient, p.Title)
		now := time.Now().Unix()

		var (
			tmdbID                             int
			tmdbTitle, tmdbYear, tmdbPosterURL string
		)
		if res.Best != nil {
			tmdbID = res.Best.ID
			tmdbTitle = res.Best.DisplayTitle()
			tmdbYear = res.Best.Year()
			tmdbPosterURL = res.Best.PosterURL()
		}
		altsJSON, _ := json.Marshal(res.Alts)

		// Sur high_confidence : on confirme direct (pas de boutons, notif silencieuse)
		// SAUF si via_bulk_import=1 → l'user veut décider chaque match → pending
		finalStatus := res.Status
		if res.Status == "high_confidence" {
			if p.BulkImport == 1 {
				finalStatus = "pending"
				res.Status = "pending"
			} else {
				finalStatus = "confirmed"
			}
		}

		// Auto-confirm rules : si l'user a configuré auto_confirm pour des teams
		// de confiance, on bypasse le pending sur les jobs qui matchent.
		if cfg.AutoConfirm.Enabled && finalStatus == "pending" {
			if applyAutoConfirm(cfg, p.Title, res.Score, p.BulkImport == 1) {
				log.Printf("[auto-post] auto-confirm rule matched → %s", p.Title)
				finalStatus = "confirmed"
				res.Status = "high_confidence" // pour la notif (notif silencieuse)
			}
		}

		// Dédup TMDB : si un autre job actif a déjà ce tmdb_id, on lie ce job
		// au parent (1 notif unique au lieu de N pour le même film).
		if tmdbID > 0 && finalStatus == "pending" {
			var parentID int64
			var parentStatus string
			qerr := db.QueryRow(`SELECT id, tmdb_status FROM jobs
				WHERE tmdb_id = ? AND id != ?
				  AND tmdb_status IN ('pending','confirmed','awaiting_manual_id','linked_pending')
				  AND COALESCE(linked_to_job_id, 0) = 0
				ORDER BY id ASC LIMIT 1`, tmdbID, p.ID).Scan(&parentID, &parentStatus)
			if qerr == nil && parentID > 0 {
				log.Printf("[auto-post] DUP %s → linked to job %d (%s)", p.Title, parentID, parentStatus)
				newStatus := "linked_pending"
				if parentStatus == "confirmed" {
					newStatus = "confirmed" // parent déjà OK → ce job aussi
				}
				_, _ = db.Exec(`UPDATE jobs SET
					tmdb_id=?, tmdb_title=?, tmdb_year=?, tmdb_score=?,
					tmdb_status=?, tmdb_poster=?, tmdb_checked_at=?, tmdb_alts_json=?,
					linked_to_job_id=?
					WHERE id=?`,
					tmdbID, tmdbTitle, tmdbYear, res.Score,
					newStatus, tmdbPosterURL, now, string(altsJSON),
					parentID, p.ID,
				)
				continue // pas de notif
			}
		}

		_, err := db.Exec(`UPDATE jobs SET
			tmdb_id=?, tmdb_title=?, tmdb_year=?, tmdb_score=?,
			tmdb_status=?, tmdb_poster=?, tmdb_checked_at=?, tmdb_alts_json=?
			WHERE id=?`,
			tmdbID, tmdbTitle, tmdbYear, res.Score,
			finalStatus, tmdbPosterURL, now, string(altsJSON), p.ID,
		)
		if err != nil {
			log.Printf("[auto-post] DB update tmdb err: %v", err)
		}

		msgID := notifyTMDBResult(cfg, db, p.ID, p.Title, res)
		if msgID > 0 {
			_, _ = db.Exec("UPDATE jobs SET telegram_message_id=? WHERE id=?", msgID, p.ID)
		}
		log.Printf("[auto-post] TMDB %s → %s (%s)", finalStatus, p.Title, res.Reason)
	}
	return nil
}

// buildTMDBButtons : construit l'inline keyboard selon le verdict.
func buildTMDBButtons(jobID int64, res TMDBResult) [][]tgInlineButton {
	id := strconv.FormatInt(jobID, 10)
	switch res.Status {
	case "high_confidence":
		// Pas de boutons : auto-confirmé
		return nil
	case "pending":
		row1 := []tgInlineButton{
			{Text: "✅ Confirmer", CallbackData: "confirm:" + id},
		}
		var altRow []tgInlineButton
		for i := range res.Alts {
			if i >= 3 {
				break
			}
			altRow = append(altRow, tgInlineButton{
				Text:         fmt.Sprintf("🔄 Alt %d", i+1),
				CallbackData: fmt.Sprintf("alt:%s:%d", id, i),
			})
		}
		row3 := []tgInlineButton{
			{Text: "❌ Skip", CallbackData: "skip:" + id},
			{Text: "✏️ ID manuel", CallbackData: "manual:" + id},
		}
		kb := [][]tgInlineButton{row1}
		if len(altRow) > 0 {
			kb = append(kb, altRow)
		}
		kb = append(kb, row3)
		return kb
	default: // no_match, error
		return [][]tgInlineButton{{
			{Text: "✏️ ID manuel", CallbackData: "manual:" + id},
			{Text: "❌ Skip", CallbackData: "skip:" + id},
		}}
	}
}

func formatTMDBNotif(release string, res TMDBResult) (caption string, silent bool) {
	switch res.Status {
	case "high_confidence":
		return fmt.Sprintf(
			"✅ <b>Match TMDB confiant</b>\n\n"+
				"<b>Release:</b> %s\n"+
				"<b>TMDB:</b> %s (%s) — score %.2f\n"+
				"<i>%s</i>",
			htmlEscape(release),
			htmlEscape(res.Best.DisplayTitle()), htmlEscape(res.Best.Year()), res.Score,
			htmlEscape(truncStr(res.Best.Overview, 200)),
		), true
	case "pending":
		alts := ""
		for _, a := range res.Alts {
			alts += fmt.Sprintf("• %s (%s)\n", htmlEscape(a.DisplayTitle()), htmlEscape(a.Year()))
		}
		return fmt.Sprintf(
			"🤔 <b>Confirmation requise</b>\n\n"+
				"<b>Release:</b> %s\n"+
				"<b>Best match:</b> %s (%s) — score %.2f\n\n"+
				"<b>Alternatives:</b>\n%s",
			htmlEscape(release),
			htmlEscape(res.Best.DisplayTitle()), htmlEscape(res.Best.Year()), res.Score,
			alts,
		), false
	case "no_match":
		return fmt.Sprintf(
			"❌ <b>Aucun match TMDB</b>\n\n"+
				"<b>Release:</b> %s\n"+
				"<b>Raison:</b> %s",
			htmlEscape(release), htmlEscape(res.Reason),
		), false
	default: // error
		return fmt.Sprintf(
			"⚠️ <b>Erreur TMDB</b>\n\n"+
				"<b>Release:</b> %s\n"+
				"<b>Raison:</b> %s",
			htmlEscape(release), htmlEscape(res.Reason),
		), false
	}
}

func truncStr(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// ---------- Main ----------

func main() {
	configPath := flag.String("config", "/etc/auto-post/config.yaml", "Path to YAML config")
	dryRun := flag.Bool("dry-run", false, "Don't submit to SABnzbd, just log")
	once := flag.Bool("once", false, "Run a single RSS poll cycle and exit (debug)")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config %s: %v", *configPath, err)
	}
	if cfg.Telegram.BotToken == "" || cfg.Telegram.ChatID == "" {
		log.Fatal("telegram.bot_token and telegram.chat_id required")
	}
	if cfg.RSS.URL == "" {
		log.Fatal("rss.url required")
	}
	if cfg.SABnzbd.URL == "" || cfg.SABnzbd.APIKey == "" {
		log.Fatal("sabnzbd.url and sabnzbd.api_key required")
	}

	db, err := openDB(cfg.Storage.DBPath)
	if err != nil {
		log.Fatalf("open db %s: %v", cfg.Storage.DBPath, err)
	}
	defer db.Close()
	initPauseState(db)
	if isPaused() {
		log.Printf("[pause] auto-post démarre EN PAUSE (flag persistant). Reprise via /admin/resume.")
	}

	tmdbClient := tmdb.NewClientWithBase(cfg.TMDB.ProxyURL)

	// Discord bot (priorité absolue sur ntfy/telegram si configuré)
	if cfg.Discord.Token != "" && cfg.Discord.ChannelID != "" {
		if err := initDiscord(cfg, db, tmdbClient); err != nil {
			log.Printf("[auto-post] Discord init err: %v (fallback ntfy/telegram)", err)
		} else {
			log.Printf("[auto-post] Discord bot connecté")
			defer closeDiscord()
			// Notif au démarrage (informe l'user que le service tourne après restart/crash)
			go func() {
				time.Sleep(2 * time.Second) // laisse Discord se stabiliser
				_ = sendDiscordSimple("🟢 Auto Post démarré",
					fmt.Sprintf("Service actif depuis %s\nIRC: %s · SAB: %s",
						time.Now().Format("15:04"),
						map[bool]string{true: "✅", false: "❌"}[cfg.IRC.Enabled],
						cfg.SABnzbd.URL),
					0x2e7d32)
			}()
		}
	}

	// Recovery : jobs en 'posting' depuis >30min sont probablement bloqués (crash
	// pendant le post). Si un nzb_id ou ddl_urls existe → marque 'posted'. Sinon
	// reset pour que pollConfirmedJobs retente.
	go func() {
		time.Sleep(10 * time.Second) // laisse les goroutines démarrer
		cutoff := time.Now().Add(-30 * time.Minute).Unix()
		rows, err := db.Query(`SELECT id, title, COALESCE(hydracker_nzb_id,0), COALESCE(hydracker_ddl_urls_json,'[]')
			FROM jobs WHERE hydracker_status='posting' AND COALESCE(hydracker_processed_at,0) < ?`, cutoff)
		if err != nil {
			return
		}
		defer rows.Close()
		var recovered, retried int
		for rows.Next() {
			var id, nzbID int64
			var title, ddlJSON string
			if rows.Scan(&id, &title, &nzbID, &ddlJSON) != nil {
				continue
			}
			if nzbID > 0 || (ddlJSON != "" && ddlJSON != "[]" && ddlJSON != "null") {
				_, _ = db.Exec("UPDATE jobs SET hydracker_status='posted' WHERE id=?", id)
				recovered++
				log.Printf("[boot-recovery] job=%d marqué posted (nzb_id=%d, ddls=%s)", id, nzbID, ddlJSON)
			} else {
				_, _ = db.Exec("UPDATE jobs SET hydracker_status=NULL, hydracker_processed_at=NULL WHERE id=?", id)
				retried++
				log.Printf("[boot-recovery] job=%d reset pour retry (rien posté)", id)
			}
		}
		if recovered+retried > 0 {
			log.Printf("[boot-recovery] %d marqués posted, %d retry", recovered, retried)
			_ = sendDiscordSimple("🔁 Recovery au boot",
				fmt.Sprintf("%d marqué(s) posté + %d retry après crash/restart", recovered, retried),
				0xed6c02)
		}
	}()

	// Hydracker client. Meta (langues + qualités) chargé lazy au 1er poll
	// (l'origine Hydracker est parfois lente / 522 Cloudflare → ne pas bloquer
	// le boot si meta indisponible).
	var hydClient *api.Client
	if cfg.Hydracker.Token != "" && cfg.Hydracker.BaseURL != "" {
		hydClient = api.NewClient(cfg.Hydracker.Token, cfg.Hydracker.BaseURL)
		log.Printf("[auto-post] Hydracker client init (meta sera chargé au 1er poll)")
	} else {
		log.Printf("[auto-post] Hydracker non configuré → post désactivé (mode RSS+SAB+TMDB seulement)")
	}

	log.Printf("[auto-post] Started. RSS=%s every %s. SAB=%s. DB=%s",
		cfg.RSS.URL, cfg.RSS.PollInterval, cfg.SABnzbd.URL, cfg.Storage.DBPath)

	if !*dryRun && !*once {
		notifyText(cfg, "🟢 Auto Post démarré",
			"Source: "+map[bool]string{true: "IRC " + cfg.IRC.Host, false: "RSS"}[cfg.IRC.Enabled]+
				"\nSAB poll: "+cfg.SABnzbd.PollInterval.String(), true)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Premier passage : pour le mode RSS, on marque tout comme vu pour ne pas
	// spammer 1000 NZBs au boot. Pour IRC c'est inutile (IRC pousse les nouveaux
	// uniquement) et ça casse les bulk imports → on skip si IRC activé.
	var seenCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM seen_items").Scan(&seenCount)
	if seenCount == 0 && !cfg.IRC.Enabled {
		log.Printf("[auto-post] DB vide : marquage des items existants comme vus (no-submit)")
		feed, err := fetchRSS(cfg.RSS.URL)
		if err != nil {
			log.Printf("[auto-post] First fetch err: %v", err)
		} else {
			now := time.Now().Unix()
			marked := 0
			for _, item := range feed.Channel.Items {
				if item.GUID == "" {
					continue
				}
				_, _ = db.Exec(
					"INSERT OR IGNORE INTO seen_items (guid, title, category, team, link, seen_at) VALUES (?, ?, ?, ?, ?, ?)",
					item.GUID, item.Title, item.Category, extractTeam(item.Title), item.Link, now,
				)
				marked++
			}
			log.Printf("[auto-post] Marked %d items as seen", marked)
		}
		if *once {
			return
		}
	}

	// Loops
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// RSS loop — désactivé si IRC enabled (IRC est la source canonique)
	if cfg.IRC.Enabled {
		log.Printf("[auto-post] IRC activé → RSS poll désactivé")
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if cfg.IRC.Enabled {
			// stub : juste attendre stop, ne rien poller
			<-stop
			return
		}
		t := time.NewTicker(cfg.RSS.PollInterval)
		defer t.Stop()
		// run once immediately (skip if seenCount==0 path already ran)
		if seenCount > 0 {
			n, k, err := processFeed(cfg, db)
			if err != nil {
				log.Printf("[auto-post] RSS err: %v", err)
			} else if n > 0 {
				log.Printf("[auto-post] RSS poll: %d new, %d submitted", n, k)
			}
			if *once {
				close(stop)
				return
			}
		}
		for {
			select {
			case <-t.C:
				n, k, err := processFeed(cfg, db)
				if err != nil {
					log.Printf("[auto-post] RSS err: %v", err)
				} else if n > 0 {
					log.Printf("[auto-post] RSS poll: %d new, %d submitted", n, k)
				}
			case <-stop:
				return
			}
		}
	}()

	// SAB poll loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(cfg.SABnzbd.PollInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if err := pollJobs(cfg, db); err != nil {
					log.Printf("[auto-post] pollJobs err: %v", err)
				}
			case <-stop:
				return
			}
		}
	}()

	// Awaiting-DL poll loop : déclenche SAB DL après confirmation user
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if err := pollAwaitingDL(cfg, db); err != nil {
					log.Printf("[auto-post] pollAwaitingDL err: %v", err)
				}
			case <-stop:
				return
			}
		}
	}()

	// TMDB poll loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(cfg.TMDB.PollInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if err := pollTMDB(cfg, db, tmdbClient); err != nil {
					log.Printf("[auto-post] pollTMDB err: %v", err)
				}
			case <-stop:
				return
			}
		}
	}()

	// Telegram bot loop : seulement si ntfy non configuré (fallback)
	if !*once && !useNtfy(cfg) && cfg.Telegram.BotToken != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runTelegramBot(cfg, db, tmdbClient, stop)
		}()
	}

	// ntfy webhook server : pour les actions des boutons (Confirmer, Skip, Alt)
	if !*once && useNtfy(cfg) && cfg.Ntfy.WebhookListen != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runNtfyWebhookServer(cfg, db, tmdbClient, stop)
		}()
	}

	// Récap hebdo Discord : tous les dimanches à 21h heure VPS (UTC), envoie un
	// résumé de la semaine.
	if !*once {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				now := time.Now()
				next := time.Date(now.Year(), now.Month(), now.Day(), 21, 0, 0, 0, now.Location())
				// Trouve le prochain dimanche 21h
				for next.Before(now) || next.Weekday() != time.Sunday {
					next = next.Add(24 * time.Hour)
				}
				wait := time.Until(next)
				log.Printf("[recap] prochain dimanche 21h dans %s", wait.Round(time.Minute))
				select {
				case <-time.After(wait):
					sendWeeklyRecap(cfg, db)
				case <-stop:
					return
				}
			}
		}()
	}

	// Catch-up scan : toutes les heures, query Newznab pour rattraper les
	// releases ratées par IRC (déco, restart...). Mode review-first.
	if cfg.IRC.Enabled && !*once {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Premier passage 10 min après boot
			firstRun := time.NewTimer(10 * time.Minute)
			defer firstRun.Stop()
			t := time.NewTicker(1 * time.Hour)
			defer t.Stop()
			run := func() {
				if err := runCatchupScan(cfg, db, tmdbClient); err != nil {
					log.Printf("[catchup] err: %v", err)
				}
			}
			for {
				select {
				case <-firstRun.C:
					run()
				case <-t.C:
					run()
				case <-stop:
					return
				}
			}
		}()
	}

	// Monitoring : check santé toutes les 10 min, alerte Discord si anomalie
	if cfg.Monitoring.Enabled && !*once {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := time.NewTicker(10 * time.Minute)
			defer t.Stop()
			// Premier check 5 min après boot
			firstRun := time.NewTimer(5 * time.Minute)
			defer firstRun.Stop()
			for {
				select {
				case <-firstRun.C:
					runMonitoring(cfg, db)
				case <-t.C:
					runMonitoring(cfg, db)
				case <-stop:
					return
				}
			}
		}()
	}

	// Cleanup loop : delete MKVs >24h post-état-final pour libérer le disque
	if !*once {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := time.NewTicker(1 * time.Hour)
			defer t.Stop()
			// Premier passage 5 min après boot (pas tout de suite, laisse le temps
			// aux autres polls de tourner)
			firstRun := time.NewTimer(5 * time.Minute)
			defer firstRun.Stop()
			for {
				select {
				case <-firstRun.C:
					if err := pollCleanup(cfg, db); err != nil {
						log.Printf("[cleanup] err: %v", err)
					}
				case <-t.C:
					if err := pollCleanup(cfg, db); err != nil {
						log.Printf("[cleanup] err: %v", err)
					}
				case <-stop:
					return
				}
			}
		}()
	}

	// IRC listener (source d'annonces — replace ou complète RSS)
	if cfg.IRC.Enabled && !*once {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runIRCListener(cfg, db, stop)
		}()
	}

	// Hydracker post loop : meta chargé paresseux (origine parfois 522 Cloudflare).
	if hydClient != nil && !*once {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var hydMeta *hydrackerMeta
			t := time.NewTicker(cfg.Hydracker.PollInterval)
			defer t.Stop()
			for {
				select {
				case <-t.C:
					if hydMeta == nil {
						m, err := loadHydrackerMeta(hydClient)
						if err != nil {
							log.Printf("[auto-post] Hydracker meta load (retry au prochain poll): %v", err)
							continue
						}
						hydMeta = m
						log.Printf("[auto-post] Hydracker meta chargé : %d langs, %d quals", len(hydMeta.langs), len(hydMeta.qualities))
					}
					if err := pollConfirmedJobs(cfg, db, hydClient, hydMeta); err != nil {
						log.Printf("[auto-post] pollConfirmedJobs err: %v", err)
					}
				case <-stop:
					return
				}
			}
		}()
	}

	if *once {
		wg.Wait()
		return
	}

	<-sigCh
	log.Println("[auto-post] Shutdown signal received, exiting.")
	close(stop)
	wg.Wait()
}
