package main

// Catch-up scan : toutes les heures, on query unfr.pw Newznab pour les 100
// dernières releases. Pour chaque release qui passe les filtres ET qu'on n'a
// PAS déjà vue (seen_items), on l'insère en mode review-first (status
// 'awaiting_dl' + notif Discord). Ça rattrape les annonces ratées pendant
// les déconnexions IRC ou les restarts du service.

import (
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"go-post-tools/internal/tmdb"
)

func runCatchupScan(cfg *Config, db *sql.DB, tmdbClient *tmdb.Client) error {
	if isPaused() {
		return nil
	}
	cats := []string{"4", "6", "55", "56", "18", "19"}
	items, _, err := searchNewznab(cfg, "", cats, 100, 0)
	if err != nil {
		return err
	}

	scanned := len(items)
	var matched, queued int
	now := time.Now().Unix()

	for _, it := range items {
		if it.GUID == "" {
			continue
		}
		// Already seen ?
		var existing string
		if err := db.QueryRow("SELECT guid FROM seen_items WHERE guid = ?", it.GUID).Scan(&existing); err == nil {
			continue
		}
		team := extractTeam(it.Title)
		pseudoItem := RSSItem{
			Title:    it.Title,
			Link:     it.Link,
			Category: it.Category,
			GUID:     it.GUID,
		}
		ok, _ := passesFilters(cfg, pseudoItem, team)
		if !ok {
			// On marque comme vu pour ne pas re-traiter à chaque scan
			_, _ = db.Exec(
				"INSERT OR IGNORE INTO seen_items (guid, title, category, team, link, seen_at) VALUES (?, ?, ?, ?, ?, ?)",
				it.GUID, it.Title, it.Category, team, it.Link, now,
			)
			continue
		}
		matched++

		// Mark as seen
		_, _ = db.Exec(
			"INSERT OR IGNORE INTO seen_items (guid, title, category, team, link, seen_at) VALUES (?, ?, ?, ?, ?, ?)",
			it.GUID, it.Title, it.Category, team, it.Link, now,
		)

		// TMDB lookup direct (mode review-first)
		res := lookupTMDB(tmdbClient, it.Title)
		var tmdbID int
		var tmdbTitle, tmdbYear, tmdbPosterURL string
		if res.Best != nil {
			tmdbID = res.Best.ID
			tmdbTitle = res.Best.DisplayTitle()
			tmdbYear = res.Best.Year()
			tmdbPosterURL = res.Best.PosterURL()
		}
		altsJSON, _ := json.Marshal(res.Alts)

		// Force pending pour catch-up (l'user décide chaque rattrapage)
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
				_, _ = db.Exec(`INSERT INTO jobs
					(guid, title, category, team, nzb_url, status,
					 tmdb_id, tmdb_title, tmdb_year, tmdb_score, tmdb_status,
					 tmdb_poster, tmdb_checked_at, tmdb_alts_json,
					 linked_to_job_id, submitted_at, via_bulk_import)
					VALUES (?, ?, ?, ?, ?, 'awaiting_dl', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
					it.GUID, it.Title, it.Category, team, it.Link,
					tmdbID, tmdbTitle, tmdbYear, res.Score, linkedStatus,
					tmdbPosterURL, now, string(altsJSON),
					parentID, now,
				)
				queued++
				log.Printf("[catchup] DUP %s → linked to job %d", it.Title, parentID)
				continue
			}
		}

		// Insert + notif Discord
		ins, _ := db.Exec(`INSERT INTO jobs
			(guid, title, category, team, nzb_url, status,
			 tmdb_id, tmdb_title, tmdb_year, tmdb_score, tmdb_status,
			 tmdb_poster, tmdb_checked_at, tmdb_alts_json,
			 submitted_at, via_bulk_import)
			VALUES (?, ?, ?, ?, ?, 'awaiting_dl', ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
			it.GUID, it.Title, it.Category, team, it.Link,
			tmdbID, tmdbTitle, tmdbYear, res.Score, tmdbInitStatus,
			tmdbPosterURL, now, string(altsJSON), now,
		)
		jobID, _ := ins.LastInsertId()
		queued++
		log.Printf("[catchup] CATCH %s [team=%s] → TMDB %s", it.Title, team, tmdbInitStatus)
		_ = notifyTMDBResult(cfg, db, jobID, it.Title, res)
		time.Sleep(1 * time.Second)
	}

	if queued > 0 {
		log.Printf("[catchup] scanned=%d matched=%d queued=%d", scanned, matched, queued)
	}
	return nil
}
