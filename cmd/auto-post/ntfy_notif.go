package main

// ntfy publishing layer + webhook server pour les actions (boutons).
// Remplace progressivement la couche Telegram. iOS reçoit notifs via app ntfy
// connectée à http://<VPS>:8000 (sur Tailnet privé).
//
// Architecture des actions :
//   - L'app ntfy iOS affiche les boutons définis dans le JSON envoyé
//   - Quand l'user tap un bouton (action http), iOS POST vers
//     http://<VPS>:<webhook_port>/{action}/{job_id}[/extra]
//   - Notre webhook server (cette file) reçoit, traite, et publie un
//     follow-up ntfy avec le résultat (ntfy n'éditant pas les notifs).

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go-post-tools/internal/tmdb"
)

// ---------- Types ----------

type ntfyAction struct {
	Action  string            `json:"action"` // "http" | "view" | "broadcast"
	Label   string            `json:"label"`
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
	Clear   bool              `json:"clear,omitempty"` // ferme la notif après tap
}

// ntfyMessage : payload JSON pour POST sur /
type ntfyMessage struct {
	Topic    string       `json:"topic"`
	Title    string       `json:"title,omitempty"`
	Message  string       `json:"message,omitempty"`
	Tags     []string     `json:"tags,omitempty"`
	Priority int          `json:"priority,omitempty"` // 1=min, 3=default, 5=max
	Click    string       `json:"click,omitempty"`
	Attach   string       `json:"attach,omitempty"` // URL d'image attachée
	Icon     string       `json:"icon,omitempty"`
	Actions  []ntfyAction `json:"actions,omitempty"`
}

// ---------- Publish helpers ----------

func ntfyPost(cfg *Config, msg ntfyMessage) error {
	if cfg.Ntfy.URL == "" {
		return fmt.Errorf("ntfy URL non configuré")
	}
	msg.Topic = cfg.Ntfy.Topic
	body, _ := json.Marshal(msg)
	req, err := http.NewRequest("POST", strings.TrimRight(cfg.Ntfy.URL, "/")+"/", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Ntfy.User != "" && cfg.Ntfy.Password != "" {
		req.SetBasicAuth(cfg.Ntfy.User, cfg.Ntfy.Password)
	}
	c := &http.Client{Timeout: 15 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ntfy HTTP %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// sendNtfy : notif texte simple
func sendNtfy(cfg *Config, title, message string, silent bool) error {
	prio := 3
	if silent {
		prio = 1
	}
	return ntfyPost(cfg, ntfyMessage{Title: title, Message: message, Priority: prio})
}

// sendNtfyPoster : notif avec poster TMDB attaché
func sendNtfyPoster(cfg *Config, title, message, posterURL string, silent bool) error {
	prio := 3
	if silent {
		prio = 1
	}
	return ntfyPost(cfg, ntfyMessage{
		Title:    title,
		Message:  message,
		Attach:   posterURL,
		Icon:     posterURL,
		Priority: prio,
	})
}

// sendNtfyWithActions : notif + boutons cliquables (chacun fait POST webhook).
// clickURL : tap sur la notif → ouvre cette URL dans Safari (typiquement la
// fiche TMDB pour vérification rapide).
// L'icône est en w185 (petit) pour s'afficher inline iOS, l'attach en w500
// (grand poster) pour le long-press / expand.
func sendNtfyWithActions(cfg *Config, title, message, posterURL, clickURL string, actions []ntfyAction, silent bool) error {
	prio := 3
	if silent {
		prio = 1
	}
	return ntfyPost(cfg, ntfyMessage{
		Title:    title,
		Message:  message,
		Attach:   posterURL,           // w500 : grand poster (long-press)
		Icon:     toSmallPoster(posterURL), // w185 : icône inline notif
		Click:    clickURL,
		Priority: prio,
		Actions:  actions,
	})
}

// toSmallPoster : convertit URL TMDB w500 → w185 pour icône notif iOS.
func toSmallPoster(url string) string {
	if url == "" {
		return ""
	}
	return strings.Replace(url, "/t/p/w500", "/t/p/w185", 1)
}

// ---------- Action button builders ----------

// httpAction : bouton qui POST vers notre webhook
func httpAction(cfg *Config, label, path string) ntfyAction {
	return ntfyAction{
		Action: "http",
		Label:  label,
		URL:    strings.TrimRight(cfg.Ntfy.WebhookBaseURL, "/") + path,
		Method: "POST",
		Headers: map[string]string{
			"X-Auto-Post-Token": cfg.Ntfy.WebhookSecret,
		},
		Clear: true,
	}
}

// buildTMDBNtfyActions : équivalent ntfy de buildTMDBButtons (Telegram)
func buildTMDBNtfyActions(cfg *Config, jobID int64, res TMDBResult) []ntfyAction {
	id := strconv.FormatInt(jobID, 10)
	switch res.Status {
	case "high_confidence":
		return nil
	case "pending":
		// ntfy limite à 3 boutons. On garde : Confirmer + Alt 1 + Skip.
		// Les autres alternatives sont dans le message texte.
		actions := []ntfyAction{
			httpAction(cfg, "✅ Confirmer", "/confirm/"+id),
		}
		if len(res.Alts) > 0 {
			actions = append(actions, httpAction(cfg, "🔄 Alt 1", fmt.Sprintf("/alt/%s/0", id)))
		}
		actions = append(actions, httpAction(cfg, "❌ Skip", "/skip/"+id))
		return actions
	default: // no_match, error
		return []ntfyAction{
			httpAction(cfg, "❌ Skip", "/skip/"+id),
		}
	}
}

// formatTMDBNtfif : titre + message pour ntfy (sans HTML, plain text)
// Inclut le lien TMDB direct (tap pour vérifier la fiche dans Safari).
func formatTMDBNtfyContent(release string, res TMDBResult) (title, message string, silent bool) {
	tmdbURL := func(id int) string { return fmt.Sprintf("https://www.themoviedb.org/movie/%d", id) }
	switch res.Status {
	case "high_confidence":
		return "✅ Match TMDB confiant",
			fmt.Sprintf("%s\n\nTMDB: %s (%s) — score %.2f\n%s",
				release, res.Best.DisplayTitle(), res.Best.Year(), res.Score, tmdbURL(res.Best.ID)),
			true
	case "pending":
		alts := ""
		for i, a := range res.Alts {
			alts += fmt.Sprintf("\nAlt %d: %s (%s) — %s", i+1, a.DisplayTitle(), a.Year(), tmdbURL(a.ID))
		}
		return "🤔 Confirmation requise",
			fmt.Sprintf("%s\n\nBest: %s (%s) — score %.2f\n%s%s",
				release, res.Best.DisplayTitle(), res.Best.Year(), res.Score, tmdbURL(res.Best.ID), alts),
			false
	case "no_match":
		return "❌ Aucun match TMDB",
			fmt.Sprintf("%s\n\nRaison: %s", release, res.Reason),
			false
	default:
		return "⚠️ Erreur TMDB",
			fmt.Sprintf("%s\n\n%s", release, res.Reason),
			false
	}
}

// ---------- Webhook server ----------

func runNtfyWebhookServer(cfg *Config, db *sql.DB, tmdbClient *tmdb.Client, stop <-chan struct{}) {
	mux := http.NewServeMux()
	// Browser-aware POST endpoints (rendent HTML après action) — utilisés par
	// la mini page web ET par les actions ntfy (qui acceptent aussi du HTML)
	mux.HandleFunc("/confirm/", makeBrowserPostHandler(cfg, db, tmdbClient, "confirm"))
	mux.HandleFunc("/skip/", makeBrowserPostHandler(cfg, db, tmdbClient, "skip"))
	mux.HandleFunc("/alt/", makeBrowserPostHandler(cfg, db, tmdbClient, "alt"))
	// /manual/{job}/{tmdb_id} : path-based, pour ntfy bouton (legacy)
	mux.HandleFunc("/manual/", makeWebhookHandler(cfg, db, tmdbClient, "manual"))
	// /manual-form/{job} : form-based avec tmdb_id en POST body
	mux.HandleFunc("/manual-form/", makeBrowserPostHandler(cfg, db, tmdbClient, "manual-form"))
	// Mini page web : tap sur la notif → Safari ouvre cette page
	mux.HandleFunc("/jobs/", func(w http.ResponseWriter, r *http.Request) {
		renderJobPage(cfg, db, w, r)
	})
	// Admin : bulk import + liste jobs
	mux.HandleFunc("/admin/import", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			handleAdminImport(cfg, db, tmdbClient, w, r)
		} else {
			renderAdminImport(cfg, w, r)
		}
	})
	mux.HandleFunc("/admin/jobs", func(w http.ResponseWriter, r *http.Request) {
		renderAdminJobs(cfg, db, w, r)
	})
	mux.HandleFunc("/admin/quick", func(w http.ResponseWriter, r *http.Request) {
		renderAdminQuick(w, r)
	})
	mux.HandleFunc("/admin/quick-import", func(w http.ResponseWriter, r *http.Request) {
		handleAdminQuickImport(cfg, db, tmdbClient, w, r)
	})
	mux.HandleFunc("/admin/search", func(w http.ResponseWriter, r *http.Request) {
		renderAdminSearch(cfg, tmdbClient, w, r)
	})
	mux.HandleFunc("/admin/search-submit", func(w http.ResponseWriter, r *http.Request) {
		handleAdminSearchSubmit(cfg, db, tmdbClient, w, r)
	})
	mux.HandleFunc("/admin/health", func(w http.ResponseWriter, r *http.Request) {
		renderAdminHealth(cfg, db, w, r)
	})
	mux.HandleFunc("/admin/stats", func(w http.ResponseWriter, r *http.Request) {
		renderAdminStats(db, w, r)
	})
	mux.HandleFunc("/admin/logs", func(w http.ResponseWriter, r *http.Request) {
		renderAdminLogs(w, r)
	})
	mux.HandleFunc("/admin/seen", func(w http.ResponseWriter, r *http.Request) {
		renderAdminSeen(db, w, r)
	})
	mux.HandleFunc("/admin/config", func(w http.ResponseWriter, r *http.Request) {
		renderAdminConfig(w, r)
	})
	mux.HandleFunc("/admin/pause", func(w http.ResponseWriter, r *http.Request) {
		handleAdminPauseToggle(db, w, r, true)
	})
	mux.HandleFunc("/admin/resume", func(w http.ResponseWriter, r *http.Request) {
		handleAdminPauseToggle(db, w, r, false)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		renderAdminHome(cfg, db, w, r)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:         cfg.Ntfy.WebhookListen,
		Handler:      formAuthMiddleware(cfg, mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	log.Printf("[webhook] listening on %s", cfg.Ntfy.WebhookListen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("[webhook] err: %v", err)
	}
}

func requireToken(cfg *Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /healthz : pas d'auth
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if cfg.Ntfy.WebhookSecret != "" && r.Header.Get("X-Auto-Post-Token") != cfg.Ntfy.WebhookSecret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func makeWebhookHandler(cfg *Config, db *sql.DB, tmdbClient *tmdb.Client, action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Path : /<action>/<job_id>[/<arg>]
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if len(parts) < 2 {
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}
		jobID, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			http.Error(w, "bad job_id", http.StatusBadRequest)
			return
		}
		switch action {
		case "confirm":
			handleNtfyConfirm(cfg, db, jobID)
		case "skip":
			handleNtfySkip(cfg, db, jobID)
		case "alt":
			if len(parts) < 3 {
				http.Error(w, "alt index manquant", http.StatusBadRequest)
				return
			}
			altIdx, _ := strconv.Atoi(parts[2])
			handleNtfyAlt(cfg, db, jobID, altIdx)
		case "manual":
			if len(parts) >= 3 {
				if tmdbID, err := strconv.Atoi(parts[2]); err == nil {
					handleNtfyManual(cfg, db, tmdbClient, jobID, tmdbID)
					_, _ = io.Copy(io.Discard, r.Body)
					w.Write([]byte("ok"))
					return
				}
			}
			http.Error(w, "tmdb_id manquant ou invalide (URL: /manual/<job>/<tmdb_id>)", http.StatusBadRequest)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Write([]byte("ok"))
	}
}

// ---------- Action handlers ----------

// countLinkedSiblings : nombre de jobs liés (linked_to_job_id=parent) en linked_pending
func countLinkedSiblings(db *sql.DB, parentID int64) int {
	var n int
	_ = db.QueryRow("SELECT COUNT(*) FROM jobs WHERE linked_to_job_id=? AND tmdb_status='linked_pending'", parentID).Scan(&n)
	return n
}

func handleNtfyConfirm(cfg *Config, db *sql.DB, jobID int64) {
	_, _ = db.Exec("UPDATE jobs SET tmdb_status='confirmed' WHERE id=?", jobID)
	// Propage aux liés : tous les linked_pending pour ce parent passent à confirmed
	siblings := countLinkedSiblings(db, jobID)
	if siblings > 0 {
		_, _ = db.Exec("UPDATE jobs SET tmdb_status='confirmed' WHERE linked_to_job_id=? AND tmdb_status='linked_pending'", jobID)
	}
	title, year := getJobTMDBInfo(db, jobID)
	suffix := ""
	if siblings > 0 {
		suffix = fmt.Sprintf(" (+%d versions liées)", siblings)
	}
	log.Printf("[webhook] confirm job=%d → %s (%s)%s", jobID, title, year, suffix)
	_ = sendNtfy(cfg, "✅ Confirmé", fmt.Sprintf("%s (%s)%s", title, year, suffix), true)
}

func handleNtfySkip(cfg *Config, db *sql.DB, jobID int64) {
	_, _ = db.Exec("UPDATE jobs SET tmdb_status='skipped' WHERE id=?", jobID)
	siblings := countLinkedSiblings(db, jobID)
	if siblings > 0 {
		_, _ = db.Exec("UPDATE jobs SET tmdb_status='skipped' WHERE linked_to_job_id=? AND tmdb_status='linked_pending'", jobID)
	}
	title := getJobTitle(db, jobID)
	suffix := ""
	if siblings > 0 {
		suffix = fmt.Sprintf(" (+%d versions liées)", siblings)
	}
	log.Printf("[webhook] skip job=%d (%s)%s", jobID, title, suffix)
	_ = sendNtfy(cfg, "❌ Skipped", title+suffix, true)
}

func handleNtfyAlt(cfg *Config, db *sql.DB, jobID int64, altIdx int) {
	altsJSON := getJobAlts(db, jobID)
	var alts []tmdb.Movie
	if err := json.Unmarshal([]byte(altsJSON), &alts); err != nil || altIdx < 0 || altIdx >= len(alts) {
		log.Printf("[webhook] alt invalid for job=%d idx=%d", jobID, altIdx)
		return
	}
	chosen := alts[altIdx]
	_, _ = db.Exec(
		"UPDATE jobs SET tmdb_id=?, tmdb_title=?, tmdb_year=?, tmdb_poster=?, tmdb_status='confirmed' WHERE id=?",
		chosen.ID, chosen.DisplayTitle(), chosen.Year(), chosen.PosterURL(), jobID,
	)
	// Propage l'alt aux liés : ils prennent la nouvelle fiche TMDB et passent confirmed
	siblings := countLinkedSiblings(db, jobID)
	if siblings > 0 {
		_, _ = db.Exec(`UPDATE jobs SET
			tmdb_id=?, tmdb_title=?, tmdb_year=?, tmdb_poster=?, tmdb_status='confirmed'
			WHERE linked_to_job_id=? AND tmdb_status='linked_pending'`,
			chosen.ID, chosen.DisplayTitle(), chosen.Year(), chosen.PosterURL(), jobID)
	}
	suffix := ""
	if siblings > 0 {
		suffix = fmt.Sprintf(" (+%d versions liées)", siblings)
	}
	log.Printf("[webhook] alt%d job=%d → %s (%s)%s", altIdx+1, jobID, chosen.DisplayTitle(), chosen.Year(), suffix)
	_ = sendNtfyPoster(cfg, fmt.Sprintf("✅ Alt %d confirmé", altIdx+1),
		fmt.Sprintf("%s (%s)%s", chosen.DisplayTitle(), chosen.Year(), suffix),
		chosen.PosterURL(), true)
}

func handleNtfyManual(cfg *Config, db *sql.DB, tmdbClient *tmdb.Client, jobID int64, tmdbID int) {
	movie, err := tmdbClient.GetByID(tmdbID, "movie")
	if err != nil || movie == nil || movie.ID == 0 {
		log.Printf("[webhook] manual lookup failed (id=%d, job=%d): %v", tmdbID, jobID, err)
		_, _ = db.Exec("UPDATE jobs SET tmdb_status='no_match' WHERE id=?", jobID)
		_ = sendNtfy(cfg, "❌ TMDB ID introuvable", fmt.Sprintf("ID %d : %v", tmdbID, err), false)
		return
	}
	_, _ = db.Exec(
		"UPDATE jobs SET tmdb_id=?, tmdb_title=?, tmdb_year=?, tmdb_poster=?, tmdb_status='confirmed' WHERE id=?",
		movie.ID, movie.DisplayTitle(), movie.Year(), movie.PosterURL(), jobID,
	)
	siblings := countLinkedSiblings(db, jobID)
	if siblings > 0 {
		_, _ = db.Exec(`UPDATE jobs SET
			tmdb_id=?, tmdb_title=?, tmdb_year=?, tmdb_poster=?, tmdb_status='confirmed'
			WHERE linked_to_job_id=? AND tmdb_status='linked_pending'`,
			movie.ID, movie.DisplayTitle(), movie.Year(), movie.PosterURL(), jobID)
	}
	suffix := ""
	if siblings > 0 {
		suffix = fmt.Sprintf(" (+%d versions liées)", siblings)
	}
	log.Printf("[webhook] manual job=%d → %s (%s) tmdb=%d%s", jobID, movie.DisplayTitle(), movie.Year(), movie.ID, suffix)
	_ = sendNtfyPoster(cfg, "✅ Confirmé manuellement",
		fmt.Sprintf("%s (%s) — TMDB %d%s", movie.DisplayTitle(), movie.Year(), movie.ID, suffix),
		movie.PosterURL(), false)
}
