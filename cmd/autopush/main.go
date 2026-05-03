// autopush : RSS NZB → notif Telegram (MVP).
//
// Polls un flux RSS toutes les N minutes, détecte les nouveaux items via
// SQLite local, applique des filtres (catégorie, team), et envoie une
// notification Telegram pour chaque release qui passe.
//
// Plus tard : forward du NZB vers Hydracker, intégration SABnzbd pour
// récupérer le MKV et déclencher torrent + DDL.
package main

import (
	"database/sql"
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
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)

type Config struct {
	RSS struct {
		URL          string        `yaml:"url"`
		PollInterval time.Duration `yaml:"poll_interval"`
	} `yaml:"rss"`
	Filters struct {
		AllowedCategories []string `yaml:"allowed_categories"`
		AllowedTeams      []string `yaml:"allowed_teams"` // vide = toutes
		BlockedTeams      []string `yaml:"blocked_teams"`
		ExcludeKeywords   []string `yaml:"exclude_keywords"`
	} `yaml:"filters"`
	Telegram struct {
		BotToken string `yaml:"bot_token"`
		ChatID   string `yaml:"chat_id"`
	} `yaml:"telegram"`
	Storage struct {
		DBPath string `yaml:"db_path"`
	} `yaml:"storage"`
}

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

// Regex pour extraire la team d'un titre type "Show.S01E01.MULTi.1080p-TEAM"
var teamRE = regexp.MustCompile(`-([A-Za-z0-9]+)$`)

func extractTeam(title string) string {
	m := teamRE.FindStringSubmatch(title)
	if len(m) < 2 {
		return ""
	}
	return m[1]
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
	if c.Storage.DBPath == "" {
		c.Storage.DBPath = "autopush.db"
	}
	return &c, nil
}

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
	`)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func fetchRSS(rssURL string) (*RSSFeed, error) {
	req, _ := http.NewRequest("GET", rssURL, nil)
	req.Header.Set("User-Agent", "go-post-tools-autopush/1.0")
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

// passesFilters renvoie (true, "") si l'item doit être notifié,
// ou (false, raison) sinon.
func passesFilters(cfg *Config, item RSSItem, team string) (bool, string) {
	// Catégorie autorisée ?
	if len(cfg.Filters.AllowedCategories) > 0 && !contains(cfg.Filters.AllowedCategories, item.Category) {
		return false, "cat exclue: " + item.Category
	}
	// Team blacklisted ?
	if len(cfg.Filters.BlockedTeams) > 0 && contains(cfg.Filters.BlockedTeams, team) {
		return false, "team blacklist: " + team
	}
	// Team whitelist (si définie)
	if len(cfg.Filters.AllowedTeams) > 0 && !contains(cfg.Filters.AllowedTeams, team) {
		return false, "team hors whitelist: " + team
	}
	// Keywords exclus
	if containsAny(item.Title, cfg.Filters.ExcludeKeywords) {
		return false, "keyword exclu"
	}
	return true, ""
}

// sendTelegram envoie un message via l'API Telegram.
// Markdown V2 nécessite échappement strict ; on reste en HTML pour simplicité.
func sendTelegram(botToken, chatID, text string) error {
	api := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	form.Set("parse_mode", "HTML")
	form.Set("disable_web_page_preview", "true")
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

func formatNotif(item RSSItem, team string) string {
	// Categorie en emoji
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
		"%s <b>Nouveau NZB</b>\n\n"+
			"<b>%s</b>\n\n"+
			"<b>📁 Catégorie:</b> %s\n"+
			"%s"+
			"<b>📅 Date:</b> %s\n\n"+
			`<a href="%s">⬇ Télécharger le NZB</a>`,
		emoji,
		htmlEscape(item.Title),
		htmlEscape(item.Category),
		teamLine,
		htmlEscape(item.PubDate),
		htmlEscape(item.Link),
	)
}

// processFeed traite tous les items du flux et envoie une notif pour chaque
// nouveau qui passe les filtres. Renvoie (nb nouveaux, nb notifiés, err).
func processFeed(cfg *Config, db *sql.DB) (int, int, error) {
	feed, err := fetchRSS(cfg.RSS.URL)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch RSS: %w", err)
	}
	now := time.Now().Unix()
	newCount := 0
	notifCount := 0

	for _, item := range feed.Channel.Items {
		if item.GUID == "" {
			continue
		}
		// Anti-doublon : déjà vu ?
		var existing string
		err := db.QueryRow("SELECT guid FROM seen_items WHERE guid = ?", item.GUID).Scan(&existing)
		if err == nil {
			continue // déjà vu
		}
		if err != sql.ErrNoRows {
			log.Printf("[autopush] DB query err: %v", err)
			continue
		}
		// Nouvel item
		newCount++
		team := extractTeam(item.Title)
		ok, reason := passesFilters(cfg, item, team)

		// Marque comme vu (qu'il passe les filtres ou non, pour éviter de le re-traiter)
		_, err = db.Exec(
			"INSERT INTO seen_items (guid, title, category, team, link, seen_at) VALUES (?, ?, ?, ?, ?, ?)",
			item.GUID, item.Title, item.Category, team, item.Link, now,
		)
		if err != nil {
			log.Printf("[autopush] DB insert err: %v", err)
			continue
		}

		if !ok {
			log.Printf("[autopush] SKIP %s (%s)", item.Title, reason)
			continue
		}

		log.Printf("[autopush] NOTIF %s [team=%s cat=%s]", item.Title, team, item.Category)
		text := formatNotif(item, team)
		if err := sendTelegram(cfg.Telegram.BotToken, cfg.Telegram.ChatID, text); err != nil {
			log.Printf("[autopush] Telegram err: %v", err)
			continue
		}
		notifCount++
		// Petit délai pour ne pas spammer Telegram (rate limit ~30 msg/sec)
		time.Sleep(100 * time.Millisecond)
	}
	return newCount, notifCount, nil
}

func main() {
	configPath := flag.String("config", "/etc/autopush/config.yaml", "Path to YAML config")
	dryRun := flag.Bool("dry-run", false, "Don't send Telegram notifs, just log")
	once := flag.Bool("once", false, "Run a single poll cycle and exit (debug)")
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

	db, err := openDB(cfg.Storage.DBPath)
	if err != nil {
		log.Fatalf("open db %s: %v", cfg.Storage.DBPath, err)
	}
	defer db.Close()

	log.Printf("[autopush] Started. Polling %s every %s. DB: %s", cfg.RSS.URL, cfg.RSS.PollInterval, cfg.Storage.DBPath)

	// Notif de démarrage
	if !*dryRun {
		_ = sendTelegram(cfg.Telegram.BotToken, cfg.Telegram.ChatID,
			"🟢 <b>Auto Push démarré</b>\n\nMonitoring du flux RSS toutes les "+cfg.RSS.PollInterval.String())
	}

	// Premier passage immédiat (mais dry-run pour le 1er = on marque tout comme vu sans spammer)
	firstRun := true

	tick := time.NewTicker(cfg.RSS.PollInterval)
	defer tick.Stop()

	// Capture Ctrl+C / SIGTERM pour shutdown propre
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	for {
		// Sur le premier run, on marque tous les items existants sans notifier
		// (sinon premier démarrage = 1000 notifs Telegram d'un coup)
		if firstRun {
			feed, err := fetchRSS(cfg.RSS.URL)
			if err != nil {
				log.Printf("[autopush] First fetch err: %v", err)
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
				log.Printf("[autopush] First run: marked %d existing items as seen (no notif)", marked)
			}
			firstRun = false
			if *once {
				return
			}
		} else {
			n, k, err := processFeed(cfg, db)
			if err != nil {
				log.Printf("[autopush] processFeed err: %v", err)
			} else if n > 0 {
				log.Printf("[autopush] poll: %d new items, %d notified", n, k)
			}
			if *once {
				return
			}
		}

		select {
		case <-tick.C:
		case <-sigCh:
			log.Println("[autopush] Shutdown signal received, exiting.")
			return
		}
	}
}
