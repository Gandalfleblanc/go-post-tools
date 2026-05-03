package main

// Bulk import depuis l'API Newznab d'unfr.pw.
// Endpoint : https://unfr.pw/api?t=search&apikey=<key>&q=&cat=<csv>&limit=100&offset=N
// Retour XML RSS Newznab. On parse, filtre par team et catégorie, et soumet
// les nouveaux à SABnzbd via le même pipeline que l'IRC listener.

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go-post-tools/internal/tmdb"
)

type newznabFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []newznabItem `xml:"item"`
	} `xml:"channel"`
}

type newznabItem struct {
	Title    string `xml:"title"`
	Link     string `xml:"link"`
	GUID     string `xml:"guid"`
	PubDate  string `xml:"pubDate"`
	Category string `xml:"category"`
}

// searchNewznab : 1 page (limit max 100). Retourne items + bool "il y a peut-être plus".
func searchNewznab(cfg *Config, q string, cats []string, limit, offset int) ([]newznabItem, bool, error) {
	return searchNewznabFull(cfg, "search", q, "", cats, limit, offset)
}

// searchNewznabByTmdbID : t=movie&tmdbid=X (unfr.pw supporte tmdbid sur movie-search).
func searchNewznabByTmdbID(cfg *Config, tmdbID int, limit, offset int) ([]newznabItem, bool, error) {
	return searchNewznabFull(cfg, "movie", "", fmt.Sprintf("%d", tmdbID), nil, limit, offset)
}

func searchNewznabFull(cfg *Config, t string, q string, tmdbID string, cats []string, limit, offset int) ([]newznabItem, bool, error) {
	params := url.Values{}
	params.Set("t", t)
	params.Set("apikey", cfg.IRC.UnfrKey)
	if q != "" {
		params.Set("q", q)
	}
	if tmdbID != "" {
		params.Set("tmdbid", tmdbID)
	}
	if len(cats) > 0 {
		params.Set("cat", strings.Join(cats, ","))
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	apiURL := "https://unfr.pw/api?" + params.Encode()

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", "go-post-tools-auto-post/1.0")
	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, false, fmt.Errorf("newznab HTTP %d: %s", resp.StatusCode, string(body[:min(200, len(body))]))
	}
	var feed newznabFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, false, fmt.Errorf("parse newznab: %w", err)
	}
	hasMore := len(feed.Channel.Items) >= limit
	return feed.Channel.Items, hasMore, nil
}

// bulkImportOptions : critères pour un run d'import
type bulkImportOptions struct {
	Categories []string // IDs Newznab (ex: ["4","6","18","19"])
	Teams      []string // whitelist team (vide = toutes whitelistées dans cfg.Filters)
	MaxPages   int      // max pages à parcourir (~100 items/page)
	MaxSubmit  int      // max items soumis à SAB (sécurité)
	DryRun     bool     // ne soumet rien, log seulement
}

// runBulkImport : lit Newznab page par page, filtre, et insère en mode
// "review-first" : status='awaiting_dl' + TMDB lookup direct + notif. La SAB
// DL ne se déclenche QUE quand l'user confirme (pollAwaitingDL).
// Retourne (scanned, matched, queued, error).
func runBulkImport(ctx context.Context, cfg *Config, db *sql.DB, tmdbClient *tmdb.Client, opts bulkImportOptions) (scanned, matched, submitted int, err error) {
	pageSize := 100
	if opts.MaxPages == 0 {
		opts.MaxPages = 5 // ~500 items max par défaut
	}
	if opts.MaxSubmit == 0 {
		opts.MaxSubmit = 20 // sécurité : pas plus de 20 NZBs en une fois
	}

	teams := opts.Teams
	if len(teams) == 0 {
		teams = cfg.Filters.AllowedTeams
	}
	cats := opts.Categories
	if len(cats) == 0 {
		// Catégories par défaut = celles de la config IRC RSS (4,6,55,56,18,19)
		cats = []string{"4", "6", "55", "56", "18", "19"}
	}

	log.Printf("[import] start cats=%v teams=%v max_pages=%d max_submit=%d dry=%v",
		cats, teams, opts.MaxPages, opts.MaxSubmit, opts.DryRun)

	for page := 0; page < opts.MaxPages; page++ {
		select {
		case <-ctx.Done():
			return scanned, matched, submitted, ctx.Err()
		default:
		}
		items, hasMore, ferr := searchNewznab(cfg, "", cats, pageSize, page*pageSize)
		if ferr != nil {
			return scanned, matched, submitted, fmt.Errorf("page %d: %w", page, ferr)
		}
		scanned += len(items)
		log.Printf("[import] page %d : %d items", page, len(items))

		for _, item := range items {
			if submitted >= opts.MaxSubmit {
				log.Printf("[import] max_submit %d atteint, stop", opts.MaxSubmit)
				return scanned, matched, submitted, nil
			}
			team := extractTeam(item.Title)
			// Filtre catégorie déjà fait par cat=, mais double-check filtre filters
			pseudoItem := RSSItem{
				Title:    item.Title,
				Link:     item.Link,
				Category: item.Category,
				GUID:     item.GUID,
			}
			ok, _ := passesFilters(cfg, pseudoItem, team)
			if !ok {
				continue
			}
			// Anti-doublon local
			var existing string
			qerr := db.QueryRow("SELECT guid FROM seen_items WHERE guid = ?", item.GUID).Scan(&existing)
			if qerr == nil {
				continue // déjà vu
			}
			matched++

			now := time.Now().Unix()

			if opts.DryRun {
				log.Printf("[import] DRY %s [%s/%s]", item.Title, item.Category, team)
				continue
			}

			_, _ = db.Exec(
				"INSERT OR IGNORE INTO seen_items (guid, title, category, team, link, seen_at) VALUES (?, ?, ?, ?, ?, ?)",
				item.GUID, item.Title, item.Category, team, item.Link, now,
			)

			// Mode review-first : TMDB lookup direct (pas de SAB DL maintenant)
			res := lookupTMDB(tmdbClient, item.Title)
			var tmdbID int
			var tmdbTitle, tmdbYear, tmdbPosterURL string
			if res.Best != nil {
				tmdbID = res.Best.ID
				tmdbTitle = res.Best.DisplayTitle()
				tmdbYear = res.Best.Year()
				tmdbPosterURL = res.Best.PosterURL()
			}
			altsJSON, _ := json.Marshal(res.Alts)

			// Statut TMDB initial : pour bulk import, tout en pending (l'user décide)
			tmdbInitStatus := res.Status
			if tmdbInitStatus == "high_confidence" {
				tmdbInitStatus = "pending"
				res.Status = "pending"
			}

			// Dédup TMDB : si un autre job actif a déjà ce tmdb_id → lier
			var parentID int64
			var parentStatus string
			if tmdbID > 0 && tmdbInitStatus == "pending" {
				qerr := db.QueryRow(`SELECT id, tmdb_status FROM jobs
					WHERE tmdb_id = ?
					  AND tmdb_status IN ('pending','confirmed','awaiting_manual_id','linked_pending')
					  AND COALESCE(linked_to_job_id, 0) = 0
					ORDER BY id ASC LIMIT 1`, tmdbID).Scan(&parentID, &parentStatus)
				if qerr == nil && parentID > 0 {
					linkedStatus := "linked_pending"
					if parentStatus == "confirmed" {
						linkedStatus = "confirmed"
					}
					res2, _ := db.Exec(`INSERT INTO jobs
						(guid, title, category, team, nzb_url, status,
						 tmdb_id, tmdb_title, tmdb_year, tmdb_score, tmdb_status,
						 tmdb_poster, tmdb_checked_at, tmdb_alts_json,
						 linked_to_job_id, submitted_at, via_bulk_import)
						VALUES (?, ?, ?, ?, ?, 'awaiting_dl', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
						item.GUID, item.Title, item.Category, team, item.Link,
						tmdbID, tmdbTitle, tmdbYear, res.Score, linkedStatus,
						tmdbPosterURL, now, string(altsJSON),
						parentID, now,
					)
					if jobID, _ := res2.LastInsertId(); jobID > 0 {
						submitted++
						log.Printf("[import] DUP %s → linked to job %d (no notif)", item.Title, parentID)
					}
					time.Sleep(200 * time.Millisecond)
					continue
				}
			}

			// Insert + notif
			ins, _ := db.Exec(`INSERT INTO jobs
				(guid, title, category, team, nzb_url, status,
				 tmdb_id, tmdb_title, tmdb_year, tmdb_score, tmdb_status,
				 tmdb_poster, tmdb_checked_at, tmdb_alts_json,
				 submitted_at, via_bulk_import)
				VALUES (?, ?, ?, ?, ?, 'awaiting_dl', ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
				item.GUID, item.Title, item.Category, team, item.Link,
				tmdbID, tmdbTitle, tmdbYear, res.Score, tmdbInitStatus,
				tmdbPosterURL, now, string(altsJSON), now,
			)
			jobID, _ := ins.LastInsertId()
			submitted++
			log.Printf("[import] QUEUE %d/%d %s [team=%s] → TMDB %s",
				submitted, opts.MaxSubmit, item.Title, team, tmdbInitStatus)

			// Notif review-first
			_ = notifyTMDBResult(cfg, db, jobID, item.Title, res)
			time.Sleep(1500 * time.Millisecond) // throttle pour pas spammer ntfy
		}

		if !hasMore {
			break
		}
		time.Sleep(1 * time.Second) // pause entre pages
	}
	log.Printf("[import] DONE scanned=%d matched=%d submitted=%d", scanned, matched, submitted)
	return scanned, matched, submitted, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
