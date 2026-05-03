package main

// Mini page web pour interaction utilisateur via Safari (tap sur notif ntfy).
// Bootstrappée par /jobs/{id} qui rend une page HTML avec poster + alternatives
// + boutons d'action. Les boutons soumettent des formulaires POST vers les
// webhooks existants (avec token embarqué).
//
// Accessible uniquement via Tailnet (UFW bloque tout sauf tailscale0).

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"go-post-tools/internal/tmdb"
)

type jobView struct {
	ID         int64
	Release    string
	BestID     int
	BestTitle  string
	BestYear   string
	BestPoster string
	BestScore  float64
	Status     string
	Alts       []altView
	Cast       []castMember
	Token      string
	Filename   string
}

type altView struct {
	Idx    int
	ID     int
	Title  string
	Year   string
	Poster string
	URL    string
}

type castMember struct {
	Name      string
	Character string
	ImageURL  string
}

// fetchMovieCast : 1 appel au proxy TMDB pour récupérer les top N acteurs.
func fetchMovieCast(proxyURL string, tmdbID, limit int) []castMember {
	if proxyURL == "" {
		proxyURL = "https://tmdb.uklm.xyz"
	}
	if !strings.HasSuffix(proxyURL, "/api") {
		proxyURL = strings.TrimRight(proxyURL, "/") + "/api"
	}
	url := fmt.Sprintf("%s?t=movie&q=%d", proxyURL, tmdbID)
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var data struct {
		Credits struct {
			Cast []struct {
				Name        string `json:"name"`
				Character   string `json:"character"`
				ProfilePath string `json:"profile_path"`
			} `json:"cast"`
		} `json:"credits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil
	}
	var out []castMember
	for i, c := range data.Credits.Cast {
		if i >= limit {
			break
		}
		img := ""
		if c.ProfilePath != "" {
			img = "https://image.tmdb.org/t/p/w185" + c.ProfilePath
		}
		out = append(out, castMember{
			Name:      c.Name,
			Character: c.Character,
			ImageURL:  img,
		})
	}
	return out
}

const jobPageTpl = `<!doctype html>
<html lang="fr">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1">
<title>Auto Post — Job {{.ID}}</title>
<style>
  :root { color-scheme: dark; }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { background: #0e0e10; color: #ececef; font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", sans-serif; padding: 16px; line-height: 1.4; }
  h1 { font-size: 18px; margin-bottom: 4px; color: #fff; }
  .release { font-size: 12px; color: #9b9ba1; word-break: break-all; margin-bottom: 16px; padding: 8px; background: #1a1a1d; border-radius: 6px; }
  .card { background: #1a1a1d; border-radius: 12px; overflow: hidden; margin-bottom: 12px; display: flex; gap: 12px; padding: 12px; }
  .card img { width: 80px; height: 120px; object-fit: cover; border-radius: 6px; flex-shrink: 0; background: #2a2a2d; }
  .card .info { flex: 1; min-width: 0; }
  .card h2 { font-size: 16px; margin-bottom: 4px; color: #fff; }
  .card .meta { font-size: 13px; color: #9b9ba1; margin-bottom: 8px; }
  .card .score { display: inline-block; background: #2e7d32; color: #fff; padding: 2px 8px; border-radius: 10px; font-size: 11px; font-weight: 600; }
  .card .score.med { background: #ed6c02; }
  .card .score.low { background: #b71c1c; }
  .actions { display: flex; flex-direction: column; gap: 8px; margin-top: 16px; }
  .btn { display: block; width: 100%; padding: 14px; border: none; border-radius: 10px; font-size: 16px; font-weight: 600; cursor: pointer; -webkit-appearance: none; }
  .btn-confirm { background: #2e7d32; color: #fff; }
  .btn-alt { background: #1e88e5; color: #fff; }
  .btn-skip { background: #424242; color: #ececef; }
  .btn-manual { background: #6a1b9a; color: #fff; }
  .btn:active { opacity: 0.7; }
  .section-title { font-size: 13px; text-transform: uppercase; color: #9b9ba1; letter-spacing: 0.5px; margin: 24px 0 8px; }
  form { display: contents; }
  .manual-form { background: #1a1a1d; border-radius: 12px; padding: 12px; margin-top: 12px; }
  .manual-form input { width: 100%; padding: 12px; background: #0e0e10; border: 1px solid #2a2a2d; border-radius: 8px; color: #fff; font-size: 16px; margin-bottom: 8px; }
  .status { padding: 12px; border-radius: 10px; margin-top: 16px; font-size: 14px; text-align: center; }
  .status.confirmed { background: #1b5e20; color: #fff; }
  .status.skipped { background: #424242; color: #ececef; }
  a { color: #64b5f6; }
</style>
</head>
<body>
<a href="/" style="display:inline-block;background:#1a1a1d;padding:6px 12px;border-radius:6px;color:#64b5f6;text-decoration:none;font-size:13px;margin-bottom:14px;">← 🏠 Accueil</a>
<h1>{{.BestTitle}} ({{.BestYear}})</h1>
<div class="release">{{.Filename}}</div>

{{if eq .Status "confirmed"}}
<div class="status confirmed">✅ Job déjà confirmé — TMDB #{{.BestID}}</div>
{{else if eq .Status "skipped"}}
<div class="status skipped">❌ Job déjà skippé</div>
{{else}}

<div class="section-title">Best match</div>
<div class="card">
  {{if .BestPoster}}<img src="{{.BestPoster}}" alt="{{.BestTitle}}">{{else}}<div style="width:80px;height:120px;background:#2a2a2d;border-radius:6px;flex-shrink:0;"></div>{{end}}
  <div class="info">
    <h2>{{.BestTitle}}</h2>
    <div class="meta">{{.BestYear}} — TMDB <code style="background:#0e0e10;padding:1px 6px;border-radius:4px;">#{{.BestID}}</code></div>
    <span class="score{{if lt .BestScore 0.85}} med{{end}}{{if lt .BestScore 0.7}} low{{end}}">Score {{printf "%.2f" .BestScore}}</span>
    <div style="margin-top:8px;font-size:12px;"><a href="https://www.themoviedb.org/movie/{{.BestID}}" target="_blank">Voir sur TMDB →</a></div>
  </div>
</div>

{{if .Cast}}
<div class="section-title">Casting principal</div>
<div style="display:flex;gap:8px;overflow-x:auto;padding-bottom:8px;">
{{range .Cast}}
<div style="flex:0 0 100px;background:#1a1a1d;border-radius:8px;padding:8px;text-align:center;">
  {{if .ImageURL}}<img src="{{.ImageURL}}" alt="{{.Name}}" style="width:84px;height:120px;object-fit:cover;border-radius:6px;background:#2a2a2d;">{{else}}<div style="width:84px;height:120px;background:#2a2a2d;border-radius:6px;display:flex;align-items:center;justify-content:center;color:#9b9ba1;font-size:24px;">?</div>{{end}}
  <div style="font-size:12px;font-weight:600;margin-top:6px;color:#fff;line-height:1.2;">{{.Name}}</div>
  <div style="font-size:10px;color:#9b9ba1;margin-top:2px;line-height:1.2;">{{.Character}}</div>
</div>
{{end}}
</div>
{{end}}

<form method="POST" action="/confirm/{{.ID}}">
  <input type="hidden" name="token" value="{{.Token}}">
  <button type="submit" class="btn btn-confirm">✅ Confirmer ce match</button>
</form>

{{if .Alts}}
<div class="section-title">Alternatives</div>
{{range .Alts}}
<div class="card">
  {{if .Poster}}<img src="{{.Poster}}" alt="{{.Title}}">{{else}}<div style="width:80px;height:120px;background:#2a2a2d;border-radius:6px;flex-shrink:0;"></div>{{end}}
  <div class="info">
    <h2>{{.Title}}</h2>
    <div class="meta">{{.Year}}</div>
    <div style="margin-top:8px;font-size:12px;"><a href="{{.URL}}" target="_blank">Voir sur TMDB →</a></div>
    <form method="POST" action="/alt/{{$.ID}}/{{.Idx}}" style="margin-top:8px;">
      <input type="hidden" name="token" value="{{$.Token}}">
      <button type="submit" class="btn btn-alt" style="padding:8px 12px;font-size:13px;">🔄 Choisir Alt {{.Idx}}</button>
    </form>
  </div>
</div>
{{end}}
{{end}}

<div class="section-title">ID TMDB manuel</div>
<form method="POST" action="/manual-form/{{.ID}}" class="manual-form">
  <input type="hidden" name="token" value="{{.Token}}">
  <input type="number" name="tmdb_id" placeholder="ID TMDB (ex: 27205)" required>
  <button type="submit" class="btn btn-manual">✏️ Utiliser cet ID</button>
</form>

<div class="actions">
  <form method="POST" action="/skip/{{.ID}}">
    <input type="hidden" name="token" value="{{.Token}}">
    <button type="submit" class="btn btn-skip">❌ Skip ce job</button>
  </form>
</div>

{{end}}
</body>
</html>
`

// ---------- Admin pages ----------

const adminImportTpl = `<!doctype html>
<html lang="fr"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Bulk import unfr.pw</title>
<style>
body{background:#0e0e10;color:#ececef;font-family:-apple-system,sans-serif;padding:16px;line-height:1.4;}
h1{font-size:20px;margin-bottom:16px;}
.hint{color:#9b9ba1;font-size:13px;margin-bottom:16px;background:#1a1a1d;padding:12px;border-radius:8px;}
form{background:#1a1a1d;padding:16px;border-radius:12px;}
label{display:block;margin-top:12px;font-size:13px;color:#9b9ba1;font-weight:600;}
input,select{width:100%;padding:12px;background:#0e0e10;border:1px solid #2a2a2d;border-radius:8px;color:#fff;font-size:16px;margin-top:4px;}
input[type=checkbox]{width:auto;margin-right:6px;}
.btn{display:block;width:100%;padding:14px;border:none;border-radius:10px;font-size:16px;font-weight:600;margin-top:16px;cursor:pointer;}
.btn-primary{background:#2e7d32;color:#fff;}
.btn-secondary{background:#424242;color:#fff;}
a{color:#64b5f6;display:block;margin-top:16px;text-align:center;font-size:13px;}
</style>
</head><body>
<a href="/" style="display:inline-block;background:#1a1a1d;padding:6px 12px;border-radius:6px;color:#64b5f6;text-decoration:none;font-size:13px;margin-bottom:14px;">← 🏠 Accueil</a>
<h1>📥 Bulk import unfr.pw → Hydracker</h1>
<div class="hint">
Importe les films d'une équipe depuis l'historique unfr.pw (rétention 1000j).<br>
Les items déjà vus sont skippés. Soumis à SAB → pipeline normal (TMDB → ntfy → Hydracker).<br>
Les filtres catégories/exclude_keywords de la config s'appliquent.
</div>
<form method="POST" action="/admin/import">
  <input type="hidden" name="token" value="{{.Token}}">
  <label>Teams (séparées par virgule, vide = whitelist config)</label>
  <input name="teams" value="{{.DefaultTeams}}" placeholder="FW,SUPPLY,Slay3R">
  <label>Catégories Newznab (IDs unfr.pw, vide = défaut films)</label>
  <input name="categories" value="" placeholder="4,6,55,56,18,19">
  <label>Max pages (100 items/page)</label>
  <input name="max_pages" type="number" value="3" min="1" max="20">
  <label>Max NZBs à soumettre à SAB</label>
  <input name="max_submit" type="number" value="10" min="1" max="100">
  <label><input type="checkbox" name="dry_run" value="1" checked> Dry-run (logue, ne soumet pas)</label>
  <button type="submit" class="btn btn-primary">🚀 Lancer l'import</button>
</form>
<a href="/admin/jobs">→ Voir les jobs en cours</a>
</body></html>
`

const adminJobsTpl = `<!doctype html>
<html lang="fr"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Jobs auto-post</title>
<style>
body{background:#0e0e10;color:#ececef;font-family:-apple-system,sans-serif;padding:16px;line-height:1.4;font-size:13px;}
h1{font-size:18px;margin-bottom:12px;}
.row{background:#1a1a1d;padding:10px;border-radius:8px;margin-bottom:6px;}
.row .title{color:#fff;font-weight:600;font-size:14px;word-break:break-all;}
.row .meta{color:#9b9ba1;font-size:11px;margin-top:4px;}
.tag{display:inline-block;padding:1px 6px;border-radius:4px;font-size:10px;font-weight:600;margin-right:4px;}
.tag.t-ready{background:#1565c0;color:#fff;}
.tag.t-downloading{background:#ed6c02;color:#fff;}
.tag.t-failed,.tag.t-submit_failed,.tag.t-no_mkv{background:#b71c1c;color:#fff;}
.tag.t-confirmed,.tag.t-posted,.tag.t-high_confidence{background:#2e7d32;color:#fff;}
.tag.t-pending{background:#6a1b9a;color:#fff;}
.tag.t-skipped,.tag.t-no_match{background:#424242;color:#fff;}
.tag.t-error,.tag.t-lookup_failed,.tag.t-no_title,.tag.t-unknown_quality,.tag.t-partial{background:#ef6c00;color:#fff;}
a{color:#64b5f6;}
</style></head><body>
<a href="/" style="display:inline-block;background:#1a1a1d;padding:6px 12px;border-radius:6px;color:#64b5f6;text-decoration:none;font-size:13px;margin-bottom:14px;">← 🏠 Accueil</a>
<h1>Jobs ({{len .Jobs}} récents)</h1>
{{range .Jobs}}
<div class="row">
  <div class="title">{{.Title}}</div>
  <div class="meta">
    <span class="tag t-{{.SAB}}">SAB:{{.SAB}}</span>
    <span class="tag t-{{.TMDB}}">TMDB:{{.TMDB}}</span>
    <span class="tag t-{{.HYD}}">HYD:{{.HYD}}</span>
    {{if .When}} · {{.When}}{{end}}
    {{if .ID}} · <a href="/jobs/{{.ID}}">→ détails</a>{{end}}
  </div>
</div>
{{else}}
<div class="row">aucun job en DB</div>
{{end}}
<a href="/admin/import">← retour import</a>
</body></html>
`

var (
	adminImportTplCompiled = template.Must(template.New("adi").Parse(adminImportTpl))
	adminJobsTplCompiled   = template.Must(template.New("adj").Parse(adminJobsTpl))
)

func renderAdminImport(cfg *Config, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	defaultTeams := strings.Join(cfg.Filters.AllowedTeams, ",")
	_ = adminImportTplCompiled.Execute(w, map[string]string{
		"Token":        cfg.Ntfy.WebhookSecret,
		"DefaultTeams": defaultTeams,
	})
}

func handleAdminImport(cfg *Config, db *sql.DB, tmdbClient *tmdb.Client, w http.ResponseWriter, r *http.Request) {
	// Accepte POST (form) ET GET (URL params) → permet bookmarks
	getVal := func(k string) string {
		if r.Method == "POST" {
			return r.Form.Get(k)
		}
		return r.URL.Query().Get(k)
	}
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "form err", 400)
			return
		}
	}
	teams := splitTrimNonEmpty(getVal("teams"))
	cats := splitTrimNonEmpty(getVal("categories"))
	maxPages := atoiSafe(getVal("max_pages"), 3)
	maxSubmit := atoiSafe(getVal("max_submit"), 10)
	dryRun := getVal("dry_run") == "1" || getVal("dry_run") == "true"

	opts := bulkImportOptions{
		Categories: cats,
		Teams:      teams,
		MaxPages:   maxPages,
		MaxSubmit:  maxSubmit,
		DryRun:     dryRun,
	}
	// Lance en background, ne bloque pas la requête
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		s, m, sub, err := runBulkImport(ctx, cfg, db, tmdbClient, opts)
		if err != nil {
			notifyText(cfg, "❌ Bulk import erreur",
				fmt.Sprintf("scanned=%d matched=%d submitted=%d err=%s", s, m, sub, err.Error()), false)
			return
		}
		mode := ""
		if dryRun {
			mode = " (DRY-RUN)"
		}
		notifyText(cfg, "📥 Bulk import terminé"+mode,
			fmt.Sprintf("Scanné: %d\nMatché: %d\nSoumis SAB: %d", s, m, sub), false)
	}()
	renderAckPage(w, "🚀", "Import lancé",
		fmt.Sprintf("teams=%v cats=%v max_pages=%d max_submit=%d dry=%v — tu recevras une notif à la fin",
			teams, cats, maxPages, maxSubmit, dryRun))
}

type adminJobRow struct {
	ID    int64
	Title string
	SAB   string
	TMDB  string
	HYD   string
	When  string
}

func renderAdminJobs(cfg *Config, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT id, title, status,
		COALESCE(tmdb_status,'-'), COALESCE(hydracker_status,'-'),
		COALESCE(submitted_at, 0)
		FROM jobs ORDER BY id DESC LIMIT 50`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var jobs []adminJobRow
	for rows.Next() {
		var j adminJobRow
		var ts int64
		if err := rows.Scan(&j.ID, &j.Title, &j.SAB, &j.TMDB, &j.HYD, &ts); err == nil {
			if ts > 0 {
				j.When = time.Unix(ts, 0).Format("01/02 15:04")
			}
			jobs = append(jobs, j)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminJobsTplCompiled.Execute(w, map[string]any{"Jobs": jobs})
}

// ---------- Quick actions ----------

const adminQuickTpl = `<!doctype html>
<html lang="fr"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Auto Post — Quick actions</title>
<style>
body{background:#0e0e10;color:#ececef;font-family:-apple-system,sans-serif;padding:16px;}
h1{font-size:20px;margin-bottom:16px;}
.card{display:block;background:#1a1a1d;padding:16px;border-radius:12px;margin-bottom:10px;text-decoration:none;color:#fff;-webkit-tap-highlight-color:transparent;}
.card:active{opacity:0.7;}
.card .t{font-size:16px;font-weight:600;}
.card .d{font-size:12px;color:#9b9ba1;margin-top:4px;}
.scan{background:#1565c0;}
.import{background:#2e7d32;}
.archive{background:#6a1b9a;}
a{color:#64b5f6;display:block;text-align:center;margin-top:16px;font-size:13px;}
</style></head><body>
<a href="/" style="display:inline-block;background:#1a1a1d;padding:6px 12px;border-radius:6px;color:#64b5f6;text-decoration:none;font-size:13px;margin-bottom:14px;">← 🏠 Accueil</a>
<h1>⚡ Quick actions</h1>
<a class="card scan" href="/admin/quick-import?dry_run=1&max_pages=10">
  <div class="t">🔍 Scan rapide (dry-run, 10 pages)</div>
  <div class="d">~1000 items, 30s, ne soumet rien — juste compte les matches</div>
</a>
<a class="card scan" href="/admin/quick-import?dry_run=1&max_pages=50">
  <div class="t">🔍 Scan complet archive (dry-run, 50 pages)</div>
  <div class="d">~5000 items, ~3 min — voir tout l'historique</div>
</a>
<a class="card import" href="/admin/quick-import?max_pages=10&max_submit=5">
  <div class="t">📥 Import léger (5 NZBs max)</div>
  <div class="d">10 pages, soumet jusqu'à 5 NZBs à SAB → notifs ntfy à confirmer</div>
</a>
<a class="card import" href="/admin/quick-import?max_pages=20&max_submit=15">
  <div class="t">📥 Import moyen (15 NZBs max)</div>
  <div class="d">20 pages, soumet jusqu'à 15 NZBs — bonne dose</div>
</a>
<a class="card archive" href="/admin/quick-import?max_pages=50&max_submit=50">
  <div class="t">📦 Import massif (50 NZBs max)</div>
  <div class="d">50 pages = archive complète, soumet jusqu'à 50 NZBs — usage gros disque</div>
</a>
<a href="/admin/search">🔍 Recherche par titre</a>
<a href="/admin/import">→ form custom</a>
<a href="/admin/jobs">→ liste des jobs</a>
</body></html>
`

var adminQuickTplCompiled = template.Must(template.New("aq").Parse(adminQuickTpl))

func renderAdminQuick(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminQuickTplCompiled.Execute(w, nil)
}

// handleAdminQuickImport : GET /admin/quick-import?max_pages=N&max_submit=M&dry_run=1
// Lance l'import en arrière-plan et affiche page d'ack. Bookmarkable.
func handleAdminQuickImport(cfg *Config, db *sql.DB, tmdbClient *tmdb.Client, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	teams := splitTrimNonEmpty(q.Get("teams"))
	cats := splitTrimNonEmpty(q.Get("categories"))
	maxPages := atoiSafe(q.Get("max_pages"), 10)
	maxSubmit := atoiSafe(q.Get("max_submit"), 5)
	dryRun := q.Get("dry_run") == "1" || q.Get("dry_run") == "true"

	opts := bulkImportOptions{
		Categories: cats,
		Teams:      teams,
		MaxPages:   maxPages,
		MaxSubmit:  maxSubmit,
		DryRun:     dryRun,
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		s, m, sub, err := runBulkImport(ctx, cfg, db, tmdbClient, opts)
		if err != nil {
			notifyText(cfg, "❌ Bulk import erreur",
				fmt.Sprintf("scanned=%d matched=%d submitted=%d err=%s", s, m, sub, err.Error()), false)
			return
		}
		mode := ""
		if dryRun {
			mode = " (DRY-RUN)"
		}
		notifyText(cfg, "📥 Bulk import terminé"+mode,
			fmt.Sprintf("Scanné: %d\nMatché: %d\nSoumis SAB: %d", s, m, sub), false)
	}()
	mode := "réel"
	if dryRun {
		mode = "dry-run"
	}
	renderAckPage(w, "🚀", "Import "+mode+" lancé",
		fmt.Sprintf("max_pages=%d max_submit=%d — notif ntfy à la fin", maxPages, maxSubmit))
}

// ---------- Home page ----------

const adminHomeTpl = `<!doctype html>
<html lang="fr"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1">
<meta http-equiv="refresh" content="60">
<title>Auto Post — Dashboard</title>
<style>
:root { color-scheme: dark; }
*{box-sizing:border-box;margin:0;padding:0;}
body{background:#0e0e10;color:#ececef;font-family:-apple-system,BlinkMacSystemFont,sans-serif;padding:16px;line-height:1.4;}
h1{font-size:22px;margin-bottom:4px;}
.sub{color:#9b9ba1;font-size:12px;margin-bottom:16px;}
.section{margin-bottom:20px;}
.section h2{font-size:13px;text-transform:uppercase;color:#9b9ba1;letter-spacing:0.5px;margin-bottom:8px;}
.stats{display:grid;grid-template-columns:repeat(2,1fr);gap:8px;}
.stat{background:#1a1a1d;padding:12px;border-radius:10px;}
.stat .v{font-size:24px;font-weight:700;color:#fff;}
.stat .l{font-size:11px;color:#9b9ba1;text-transform:uppercase;}
.stat.green .v{color:#66bb6a;}
.stat.red .v{color:#ef5350;}
.stat.yellow .v{color:#ffa726;}
.stat.blue .v{color:#42a5f5;}
.card{display:block;background:#1a1a1d;padding:14px;border-radius:12px;margin-bottom:8px;text-decoration:none;color:#fff;-webkit-tap-highlight-color:transparent;}
.card:active{opacity:0.7;}
.card .t{font-size:15px;font-weight:600;}
.card .d{font-size:12px;color:#9b9ba1;margin-top:3px;}
.card.scan{background:linear-gradient(135deg,#0d47a1,#1565c0);}
.card.import{background:linear-gradient(135deg,#1b5e20,#2e7d32);}
.card.archive{background:linear-gradient(135deg,#4a148c,#6a1b9a);}
.card.search{background:linear-gradient(135deg,#01579b,#0288d1);}
.search-form{background:#1a1a1d;padding:12px;border-radius:12px;display:flex;gap:8px;}
.search-form input{flex:1;padding:12px;background:#0e0e10;border:1px solid #2a2a2d;border-radius:8px;color:#fff;font-size:16px;}
.search-form button{padding:12px 16px;border:none;border-radius:8px;background:#1565c0;color:#fff;font-weight:600;cursor:pointer;}
.links{display:grid;grid-template-columns:repeat(2,1fr);gap:8px;}
.links a{background:#1a1a1d;padding:10px;border-radius:8px;color:#64b5f6;text-decoration:none;font-size:13px;text-align:center;}
.last-jobs{background:#1a1a1d;padding:10px;border-radius:10px;}
.last-jobs .row{padding:8px 0;border-bottom:1px solid #2a2a2d;font-size:12px;}
.last-jobs .row:last-child{border-bottom:none;}
.last-jobs .title{color:#fff;font-weight:600;word-break:break-all;}
.last-jobs .meta{color:#9b9ba1;margin-top:2px;font-size:10px;}
.tag{display:inline-block;padding:1px 6px;border-radius:4px;font-size:9px;font-weight:600;margin-right:4px;background:#2a2a2d;}
.tag.t-confirmed,.tag.t-posted{background:#2e7d32;color:#fff;}
.tag.t-pending{background:#6a1b9a;color:#fff;}
.tag.t-skipped{background:#424242;color:#fff;}
.tag.t-failed,.tag.t-no_match{background:#b71c1c;color:#fff;}
.tag.t-downloading{background:#ed6c02;color:#fff;}
.tag.t-ready,.tag.t-awaiting_dl{background:#1565c0;color:#fff;}
.tag.t-dup_skipped,.tag.t-quality_skipped{background:#424242;color:#9b9ba1;}
.refresh{color:#9b9ba1;font-size:10px;text-align:center;margin-top:12px;}
</style></head><body>
<h1>🚀 Auto Post Dashboard</h1>
<div class="sub">{{.Hostname}} · IRC: {{if .IRCActive}}🟢 actif{{else}}🔴 down{{end}} · Discord: {{if .DiscordActive}}🟢 connecté{{else}}🔴 down{{end}} · État: {{if .Paused}}⏸ <b style="color:#ed6c02;">en pause</b>{{else}}▶ <b style="color:#66bb6a;">en marche</b>{{end}}</div>

<div class="section">
  <h2>🎛 Contrôle service</h2>
  <div style="display:flex;gap:10px;flex-wrap:wrap;">
    {{if .Paused}}
    <form method="POST" action="/admin/resume" style="margin:0;">
      <input type="hidden" name="token" value="{{.Token}}">
      <button type="submit" style="background:#2e7d32;color:#fff;border:none;border-radius:8px;padding:14px 22px;font-size:15px;font-weight:700;cursor:pointer;">▶ Démarrer Auto Post</button>
    </form>
    <div style="color:#9b9ba1;font-size:13px;align-self:center;">Tous les polls sont gelés (IRC, RSS, SAB, TMDB, Hydracker post, catchup).</div>
    {{else}}
    <form method="POST" action="/admin/pause" style="margin:0;" onsubmit="return confirm('Mettre auto-post en pause ?');">
      <input type="hidden" name="token" value="{{.Token}}">
      <button type="submit" style="background:#b71c1c;color:#fff;border:none;border-radius:8px;padding:14px 22px;font-size:15px;font-weight:700;cursor:pointer;">⏸ Arrêter Auto Post</button>
    </form>
    <div style="color:#9b9ba1;font-size:13px;align-self:center;">Pause logicielle : binaire vivant (web UI accessible), aucun nouveau job traité.</div>
    {{end}}
  </div>
</div>

<div class="section">
  <h2>📊 État</h2>
  <div class="stats">
    <div class="stat blue"><div class="v">{{.PendingCount}}</div><div class="l">Pending (à valider)</div></div>
    <div class="stat green"><div class="v">{{.PostedToday}}</div><div class="l">Posté(s) Hydracker (24h)</div></div>
    <div class="stat blue"><div class="v">{{.HandledToday}}</div><div class="l">Traités (24h: posted/dup/skipped)</div></div>
    <div class="stat yellow"><div class="v">{{.DownloadingCount}}</div><div class="l">SAB DL en cours</div></div>
    <div class="stat"><div class="v">{{.DiskFree}}</div><div class="l">Disque libre</div></div>
  </div>
  {{if .PostedTitles}}
  <div style="margin-top:8px;background:#1a1a1d;padding:10px;border-radius:8px;">
    <div style="font-size:11px;color:#9b9ba1;text-transform:uppercase;margin-bottom:6px;">📤 Postés sur Hydracker (24h)</div>
    {{range .PostedTitles}}
    <div style="font-size:12px;color:#66bb6a;padding:3px 0;word-break:break-all;">✓ {{.}}</div>
    {{end}}
  </div>
  {{end}}
</div>

<div class="section">
  <h2>🔍 Recherche rapide</h2>
  <form method="GET" action="/admin/search" class="search-form">
    <input name="q" placeholder="Titre du film…" autocomplete="off">
    <button type="submit">Go</button>
  </form>
</div>

<div class="section">
  <h2>⚡ Quick actions</h2>
  <a class="card scan" href="/admin/quick-import?dry_run=1&max_pages=10">
    <div class="t">🔍 Scan rapide (dry-run, 10 pages)</div>
    <div class="d">~1000 items, 30s, ne soumet rien</div>
  </a>
  <a class="card import" href="/admin/quick-import?max_pages=10&max_submit=5">
    <div class="t">📥 Import léger (5 NZBs max)</div>
    <div class="d">10 pages, soumet jusqu'à 5 → notifs Discord</div>
  </a>
  <a class="card import" href="/admin/quick-import?max_pages=20&max_submit=15">
    <div class="t">📥 Import moyen (15 NZBs max)</div>
    <div class="d">20 pages</div>
  </a>
  <a class="card archive" href="/admin/quick-import?max_pages=50&max_submit=50">
    <div class="t">📦 Import massif (50 NZBs max)</div>
    <div class="d">Archive complète (5000 items)</div>
  </a>
</div>

{{if .SABQueue}}
<div class="section">
  <h2>📥 SAB en cours</h2>
  <div class="last-jobs">
    {{range .SABQueue}}
    <div class="row">
      <div class="title">{{.Filename}}</div>
      <div class="meta">
        <span class="tag t-downloading">{{.Percentage}}%</span>
        · {{.Size}} · ETA {{.TimeLeft}} · {{.Speed}}/s
      </div>
    </div>
    {{end}}
  </div>
</div>
{{end}}

{{if .RecentJobs}}
<div class="section">
  <h2>🕐 Derniers jobs</h2>
  <div class="last-jobs">
    {{range .RecentJobs}}
    <div class="row">
      <div class="title">{{.Title}}</div>
      <div class="meta">
        <span class="tag t-{{.SAB}}">SAB:{{.SAB}}</span>
        <span class="tag t-{{.TMDB}}">TMDB:{{.TMDB}}</span>
        <span class="tag t-{{.HYD}}">HYD:{{.HYD}}</span>
        {{if .When}}· {{.When}}{{end}}
      </div>
    </div>
    {{end}}
  </div>
</div>
{{end}}

<div class="section">
  <h2>🔗 Pages internes</h2>
  <div class="links">
    <a href="/admin/quick">⚡ Quick actions</a>
    <a href="/admin/search">🔍 Recherche</a>
    <a href="/admin/jobs">📋 Tous les jobs</a>
    <a href="/admin/health">🩺 Health</a>
    <a href="/admin/stats">📊 Stats</a>
    <a href="/admin/logs">📜 Logs</a>
    <a href="/admin/seen">👁 Seen items</a>
    <a href="/admin/config">⚙️ Config</a>
  </div>
</div>

<div class="section">
  <h2>🌐 Liens externes</h2>
  <div class="links">
    <a href="https://hydracker.com" target="_blank">Hydracker</a>
    <a href="https://www.themoviedb.org" target="_blank">TMDB</a>
    <a href="https://unfr.pw" target="_blank">unfr.pw</a>
    <a href="http://100.72.205.55:8080" target="_blank">SABnzbd UI</a>
    <a href="https://1fichier.com/console/" target="_blank">1Fichier console</a>
    <a href="https://send.now/console" target="_blank">Send.now</a>
    <a href="https://eweka.nl/en/account" target="_blank">Eweka account</a>
    <a href="https://discord.com/channels/@me" target="_blank">Discord</a>
  </div>
</div>

<div class="refresh">↻ auto-refresh 60s</div>
</body></html>
`

var adminHomeTplCompiled = template.Must(template.New("ah").Parse(adminHomeTpl))

func renderAdminHome(cfg *Config, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	type sabQueueRow struct {
		Filename   string
		Percentage string
		Size       string
		TimeLeft   string
		Speed      string
	}
	type homeData struct {
		Hostname         string
		IRCActive        bool
		DiscordActive    bool
		Paused           bool
		Token            string
		PendingCount     int
		PostedToday      int
		HandledToday     int
		DownloadingCount int
		DiskFree         string
		PostedTitles     []string
		SABQueue         []sabQueueRow
		RecentJobs       []adminJobRow
	}
	d := homeData{
		Hostname:      "ov-ead587",
		IRCActive:     cfg.IRC.Enabled,
		DiscordActive: globalDiscord != nil,
		Paused:        isPaused(),
		Token:         cfg.Ntfy.WebhookSecret,
	}

	// Counts
	_ = db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE tmdb_status='pending'`).Scan(&d.PendingCount)
	cutoff24h := time.Now().Add(-24 * time.Hour).Unix()
	_ = db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE hydracker_status='posted' AND hydracker_processed_at >= ?`,
		cutoff24h).Scan(&d.PostedToday)
	_ = db.QueryRow(`SELECT COUNT(*) FROM jobs
		WHERE hydracker_processed_at >= ?
		  AND hydracker_status IN ('posted','partial','dup_skipped','quality_skipped','no_title','unknown_quality','lookup_failed','failed')`,
		cutoff24h).Scan(&d.HandledToday)
	_ = db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE status='downloading'`).Scan(&d.DownloadingCount)

	// SAB queue : fetch live state pour montrer DLs en cours
	if d.DownloadingCount > 0 {
		sabAPIClient := &http.Client{Timeout: 5 * time.Second}
		if resp, err := sabAPIClient.Get(cfg.SABnzbd.URL + "/api?mode=queue&output=json&apikey=" + cfg.SABnzbd.APIKey); err == nil {
			defer resp.Body.Close()
			var sq struct {
				Queue struct {
					Speed string `json:"speed"`
					Slots []struct {
						Filename   string `json:"filename"`
						Percentage string `json:"percentage"`
						MB         string `json:"mb"`
						TimeLeft   string `json:"timeleft"`
					} `json:"slots"`
				} `json:"queue"`
			}
			if json.NewDecoder(resp.Body).Decode(&sq) == nil {
				for i, s := range sq.Queue.Slots {
					if i >= 5 {
						break
					}
					mbF, _ := strconv.ParseFloat(s.MB, 64)
					sz := fmt.Sprintf("%.0f MB", mbF)
					if mbF >= 1024 {
						sz = fmt.Sprintf("%.2f GB", mbF/1024)
					}
					tl := s.TimeLeft
					if tl == "" {
						tl = "?"
					}
					name := s.Filename
					if len(name) > 60 {
						name = name[:57] + "…"
					}
					d.SABQueue = append(d.SABQueue, sabQueueRow{
						Filename:   name,
						Percentage: s.Percentage,
						Size:       sz,
						TimeLeft:   tl,
						Speed:      sq.Queue.Speed,
					})
				}
			}
		}
	}

	// Liste des titres postés (24h)
	if titleRows, err := db.Query(`SELECT title FROM jobs
		WHERE hydracker_status='posted' AND hydracker_processed_at >= ?
		ORDER BY hydracker_processed_at DESC LIMIT 20`, cutoff24h); err == nil {
		defer titleRows.Close()
		for titleRows.Next() {
			var t string
			if err := titleRows.Scan(&t); err == nil {
				d.PostedTitles = append(d.PostedTitles, t)
			}
		}
	}

	// Disque
	d.DiskFree = "?"
	if out, err := exec.Command("df", "-BG", "/").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 1 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 4 {
				d.DiskFree = fields[3]
			}
		}
	}

	// Derniers jobs
	rows, err := db.Query(`SELECT id, title, status,
		COALESCE(tmdb_status,'-'), COALESCE(hydracker_status,'-'),
		COALESCE(submitted_at, 0)
		FROM jobs ORDER BY id DESC LIMIT 8`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var j adminJobRow
			var ts int64
			if err := rows.Scan(&j.ID, &j.Title, &j.SAB, &j.TMDB, &j.HYD, &ts); err == nil {
				if ts > 0 {
					j.When = time.Unix(ts, 0).Format("01/02 15:04")
				}
				d.RecentJobs = append(d.RecentJobs, j)
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminHomeTplCompiled.Execute(w, d)
}

// ---------- Search page ----------

const adminSearchTpl = `<!doctype html>
<html lang="fr"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Recherche unfr.pw</title>
<style>
body{background:#0e0e10;color:#ececef;font-family:-apple-system,sans-serif;padding:16px;line-height:1.4;}
h1{font-size:20px;margin-bottom:12px;}
form{background:#1a1a1d;padding:16px;border-radius:12px;margin-bottom:16px;}
label{display:block;margin-top:8px;font-size:13px;color:#9b9ba1;font-weight:600;}
input{width:100%;padding:12px;background:#0e0e10;border:1px solid #2a2a2d;border-radius:8px;color:#fff;font-size:16px;margin-top:4px;}
.btn{display:block;width:100%;padding:14px;border:none;border-radius:10px;font-size:16px;font-weight:600;margin-top:12px;cursor:pointer;background:#1565c0;color:#fff;}
.result{background:#1a1a1d;padding:12px;border-radius:10px;margin-bottom:8px;}
.result .title{font-size:14px;font-weight:600;color:#fff;word-break:break-all;}
.result .meta{font-size:12px;color:#9b9ba1;margin-top:4px;}
.result form{padding:0;background:transparent;margin:8px 0 0;}
.result button{padding:8px 14px;background:#2e7d32;color:#fff;border:none;border-radius:8px;font-weight:600;cursor:pointer;}
a{color:#64b5f6;text-decoration:none;}
.empty{color:#9b9ba1;text-align:center;padding:32px;}
.tag{display:inline-block;padding:1px 6px;border-radius:4px;font-size:10px;font-weight:600;margin-right:4px;background:#2a2a2d;}
.tag.team-ok{background:#2e7d32;}
.tag.team-no{background:#b71c1c;}
</style></head><body>
<a href="/" style="display:inline-block;background:#1a1a1d;padding:6px 12px;border-radius:6px;color:#64b5f6;text-decoration:none;font-size:13px;margin-bottom:14px;">← 🏠 Accueil</a>
<h1>🔍 Recherche unfr.pw</h1>
<form method="GET" action="/admin/search">
  <label>Titre (ou laisse vide si tu utilises l'ID TMDB)</label>
  <input name="q" value="{{.Query}}" placeholder="ex: Inception">
  <label>Année (optionnelle, pour préciser le titre)</label>
  <input name="year" value="{{.Year}}" placeholder="ex: 2010">
  <label>OU ID TMDB (le système récupère le titre auto)</label>
  <input name="tmdb_id" value="{{.TMDBID}}" placeholder="ex: 27205" inputmode="numeric">
  <button type="submit" class="btn">Rechercher</button>
</form>
{{if .ResolvedTitle}}<div style="font-size:12px;color:#9b9ba1;margin-bottom:8px;">🔎 TMDB ID résolu : <strong style="color:#fff;">{{.ResolvedTitle}} ({{.ResolvedYear}})</strong></div>{{end}}

{{if .Searched}}
  {{if .Results}}
    <h2 style="font-size:14px;margin-bottom:8px;color:#9b9ba1;">{{len .Results}} résultats</h2>
    {{range .Results}}
    <div class="result">
      <div class="title">{{.Title}}</div>
      <div class="meta">
        <span class="tag">{{.Category}}</span>
        {{if .Team}}<span class="tag team-{{if .TeamOK}}ok{{else}}no{{end}}">{{.Team}}</span>{{end}}
      </div>
      <form method="POST" action="/admin/search-submit">
        <input type="hidden" name="token" value="{{$.Token}}">
        <input type="hidden" name="guid" value="{{.GUID}}">
        <input type="hidden" name="title" value="{{.Title}}">
        <input type="hidden" name="link" value="{{.Link}}">
        <input type="hidden" name="category" value="{{.Category}}">
        <button type="submit">📥 Submit (review-first)</button>
      </form>
    </div>
    {{end}}
  {{else}}
    <div class="empty">Aucun résultat pour "{{.Query}}"</div>
  {{end}}
{{end}}
<div style="margin-top:16px;text-align:center;"><a href="/admin/quick">← Quick actions</a></div>
</body></html>
`

type searchResult struct {
	Title    string
	Link     string
	Category string
	GUID     string
	Team     string
	TeamOK   bool
}

var adminSearchTplCompiled = template.Must(template.New("as").Parse(adminSearchTpl))

func renderAdminSearch(cfg *Config, tmdbClient *tmdb.Client, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	year := r.URL.Query().Get("year")
	tmdbIDStr := r.URL.Query().Get("tmdb_id")
	resolvedTitle := ""
	resolvedYear := ""

	// Si TMDB ID fourni, on resolve le titre via le proxy TMDB
	if tmdbIDStr != "" {
		if id, err := strconv.Atoi(strings.TrimSpace(tmdbIDStr)); err == nil && id > 0 {
			if movie, err := tmdbClient.GetByID(id, "movie"); err == nil && movie != nil {
				resolvedTitle = movie.DisplayTitle()
				resolvedYear = movie.Year()
				if q == "" {
					q = resolvedTitle
				}
				if year == "" {
					year = resolvedYear
				}
			}
		}
	}

	query := strings.TrimSpace(q)
	if year != "" {
		query += " " + year
	}

	data := map[string]any{
		"Token":         cfg.Ntfy.WebhookSecret,
		"Query":         q,
		"Year":          year,
		"TMDBID":        tmdbIDStr,
		"ResolvedTitle": resolvedTitle,
		"ResolvedYear":  resolvedYear,
		"Searched":      query != "" || tmdbIDStr != "",
		"Results":       []searchResult{},
	}

	// Si TMDB ID fourni, on utilise la recherche directe par tmdbid
	// (unfr.pw supporte t=movie&tmdbid=X — plus précis que la recherche texte)
	var items []newznabItem
	var searchErr error
	if tmdbIDStr != "" {
		if id, err := strconv.Atoi(strings.TrimSpace(tmdbIDStr)); err == nil && id > 0 {
			items, _, searchErr = searchNewznabByTmdbID(cfg, id, 100, 0)
		}
	} else if query != "" {
		items, _, searchErr = searchNewznab(cfg, query, []string{"4", "6", "55", "56", "18", "19"}, 50, 0)
	}
	if searchErr == nil && (tmdbIDStr != "" || query != "") {
		allowedTeams := map[string]bool{}
		for _, t := range cfg.Filters.AllowedTeams {
			allowedTeams[strings.ToLower(t)] = true
		}
		results := make([]searchResult, 0, len(items))
		for _, it := range items {
			team := extractTeam(it.Title)
			results = append(results, searchResult{
				Title:    it.Title,
				Link:     it.Link,
				Category: it.Category,
				GUID:     it.GUID,
				Team:     team,
				TeamOK:   team != "" && allowedTeams[strings.ToLower(team)],
			})
		}
		data["Results"] = results
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminSearchTplCompiled.Execute(w, data)
}

// handleAdminSearchSubmit : soumet 1 release sélectionnée par l'user au pipeline
// review-first (TMDB lookup direct → notif Discord → user confirm → DL → post).
func handleAdminSearchSubmit(cfg *Config, db *sql.DB, tmdbClient *tmdb.Client, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form err", 400)
		return
	}
	guid := r.Form.Get("guid")
	title := r.Form.Get("title")
	link := r.Form.Get("link")
	category := r.Form.Get("category")
	if guid == "" || title == "" || link == "" {
		http.Error(w, "fields manquants", 400)
		return
	}
	team := extractTeam(title)
	now := time.Now().Unix()

	// Anti-doublon local
	var existing string
	if err := db.QueryRow("SELECT guid FROM seen_items WHERE guid=?", guid).Scan(&existing); err == nil {
		renderAckPage(w, "⏭", "Déjà vu", title)
		return
	}
	_, _ = db.Exec(
		"INSERT OR IGNORE INTO seen_items (guid, title, category, team, link, seen_at) VALUES (?, ?, ?, ?, ?, ?)",
		guid, title, category, team, link, now,
	)

	// TMDB lookup direct
	res := lookupTMDB(tmdbClient, title)
	var tmdbID int
	var tmdbTitle, tmdbYear, tmdbPosterURL string
	if res.Best != nil {
		tmdbID = res.Best.ID
		tmdbTitle = res.Best.DisplayTitle()
		tmdbYear = res.Best.Year()
		tmdbPosterURL = res.Best.PosterURL()
	}
	altsJSON, _ := json.Marshal(res.Alts)

	tmdbInitStatus := res.Status
	if tmdbInitStatus == "high_confidence" {
		tmdbInitStatus = "pending"
		res.Status = "pending"
	}

	// Insert job en review-first
	ins, _ := db.Exec(`INSERT INTO jobs
		(guid, title, category, team, nzb_url, status,
		 tmdb_id, tmdb_title, tmdb_year, tmdb_score, tmdb_status,
		 tmdb_poster, tmdb_checked_at, tmdb_alts_json,
		 submitted_at, via_bulk_import)
		VALUES (?, ?, ?, ?, ?, 'awaiting_dl', ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		guid, title, category, team, link,
		tmdbID, tmdbTitle, tmdbYear, res.Score, tmdbInitStatus,
		tmdbPosterURL, now, string(altsJSON), now,
	)
	jobID, _ := ins.LastInsertId()
	_ = notifyTMDBResult(cfg, db, jobID, title, res)
	renderAckPage(w, "🚀", "Soumis pour validation",
		fmt.Sprintf("%s\n\nNotif Discord envoyée — tape Confirmer/Skip", title))
}

// ---------- Health page ----------

const adminHealthTpl = `<!doctype html>
<html lang="fr"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="refresh" content="30">
<title>Health — Auto Post</title>
<style>
:root{color-scheme:dark;}*{box-sizing:border-box;margin:0;padding:0;}
body{background:#0e0e10;color:#ececef;font-family:-apple-system,sans-serif;padding:16px;line-height:1.4;}
h1{font-size:20px;margin-bottom:16px;}
.check{background:#1a1a1d;padding:14px;border-radius:10px;margin-bottom:8px;display:flex;align-items:center;gap:12px;}
.dot{width:14px;height:14px;border-radius:50%;flex-shrink:0;}
.ok{background:#2e7d32;}.fail{background:#b71c1c;}.warn{background:#ed6c02;}
.info{flex:1;}
.info .name{font-size:14px;font-weight:600;color:#fff;}
.info .detail{font-size:12px;color:#9b9ba1;margin-top:2px;}
.refresh{color:#9b9ba1;font-size:10px;text-align:center;margin-top:16px;}
</style></head><body>
<a href="/" style="display:inline-block;background:#1a1a1d;padding:6px 12px;border-radius:6px;color:#64b5f6;text-decoration:none;font-size:13px;margin-bottom:14px;">← 🏠 Accueil</a>
<h1>🩺 Health checks</h1>
{{range .Checks}}
<div class="check">
  <div class="dot {{.Status}}"></div>
  <div class="info"><div class="name">{{.Name}}</div><div class="detail">{{.Detail}}</div></div>
</div>
{{end}}
<div class="refresh">↻ auto-refresh 30s</div>
</body></html>`

type healthCheck struct {
	Name   string
	Status string // "ok" | "fail" | "warn"
	Detail string
}

var adminHealthTplCompiled = template.Must(template.New("hc").Parse(adminHealthTpl))

func renderAdminHealth(cfg *Config, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var checks []healthCheck

	// Discord
	if globalDiscord != nil && globalDiscord.session != nil && globalDiscord.session.State != nil {
		checks = append(checks, healthCheck{Name: "Discord bot", Status: "ok",
			Detail: fmt.Sprintf("Connecté en tant que %s", globalDiscord.session.State.User.Username)})
	} else {
		checks = append(checks, healthCheck{Name: "Discord bot", Status: "fail", Detail: "non connecté"})
	}

	// IRC
	if cfg.IRC.Enabled {
		checks = append(checks, healthCheck{Name: "IRC listener", Status: "ok",
			Detail: cfg.IRC.Host + ":" + strconv.Itoa(cfg.IRC.Port) + " — " + cfg.IRC.Nick})
	} else {
		checks = append(checks, healthCheck{Name: "IRC listener", Status: "warn", Detail: "désactivé en config"})
	}

	// SAB
	{
		c := &http.Client{Timeout: 5 * time.Second}
		resp, err := c.Get(cfg.SABnzbd.URL + "/api?mode=version&apikey=" + cfg.SABnzbd.APIKey + "&output=json")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			checks = append(checks, healthCheck{Name: "SABnzbd", Status: "ok", Detail: cfg.SABnzbd.URL})
		} else {
			det := "non joignable"
			if err != nil {
				det = err.Error()
			}
			checks = append(checks, healthCheck{Name: "SABnzbd", Status: "fail", Detail: det})
		}
	}

	// Hydracker
	{
		req, _ := http.NewRequest("GET", cfg.Hydracker.BaseURL+"/meta/quals", nil)
		req.Header.Set("Authorization", "Bearer "+cfg.Hydracker.Token)
		c := &http.Client{Timeout: 8 * time.Second}
		resp, err := c.Do(req)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			checks = append(checks, healthCheck{Name: "Hydracker API", Status: "ok", Detail: cfg.Hydracker.BaseURL})
		} else {
			det := "non joignable"
			if err != nil {
				det = err.Error()
			} else if resp != nil {
				det = fmt.Sprintf("HTTP %d", resp.StatusCode)
				resp.Body.Close()
			}
			checks = append(checks, healthCheck{Name: "Hydracker API", Status: "fail", Detail: det})
		}
	}

	// TMDB proxy
	{
		c := &http.Client{Timeout: 5 * time.Second}
		resp, err := c.Get("https://tmdb.uklm.xyz/health")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			checks = append(checks, healthCheck{Name: "TMDB proxy", Status: "ok", Detail: "tmdb.uklm.xyz"})
		} else {
			checks = append(checks, healthCheck{Name: "TMDB proxy", Status: "warn", Detail: "non joignable (pas critique si IRC down aussi)"})
		}
	}

	// Disk
	{
		out, _ := exec.Command("df", "-BG", "/").Output()
		lines := strings.Split(string(out), "\n")
		if len(lines) > 1 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 5 {
				usePct, _ := strconv.Atoi(strings.TrimSuffix(fields[4], "%"))
				status := "ok"
				if usePct > 90 {
					status = "fail"
				} else if usePct > 75 {
					status = "warn"
				}
				checks = append(checks, healthCheck{Name: "Disque /", Status: status,
					Detail: fmt.Sprintf("%s utilisé / %s total — %s libre", fields[2], fields[1], fields[3])})
			}
		}
	}

	// Mémoire
	{
		out, _ := exec.Command("free", "-m").Output()
		lines := strings.Split(string(out), "\n")
		if len(lines) > 1 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 4 {
				checks = append(checks, healthCheck{Name: "Mémoire", Status: "ok",
					Detail: fmt.Sprintf("%s MB utilisé / %s MB total", fields[2], fields[1])})
			}
		}
	}

	// auto-post.service uptime
	{
		out, _ := exec.Command("systemctl", "show", "auto-post.service", "--property=ActiveEnterTimestamp", "--no-pager").Output()
		uptime := strings.TrimPrefix(strings.TrimSpace(string(out)), "ActiveEnterTimestamp=")
		checks = append(checks, healthCheck{Name: "auto-post.service", Status: "ok",
			Detail: "started " + uptime})
	}

	// Jobs DB
	{
		var n int
		_ = db.QueryRow("SELECT COUNT(*) FROM jobs").Scan(&n)
		checks = append(checks, healthCheck{Name: "DB jobs", Status: "ok",
			Detail: fmt.Sprintf("%d jobs au total", n)})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminHealthTplCompiled.Execute(w, map[string]any{"Checks": checks})
}

// ---------- Stats page ----------

const adminStatsTpl = `<!doctype html>
<html lang="fr"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Stats — Auto Post</title>
<style>
:root{color-scheme:dark;}*{box-sizing:border-box;margin:0;padding:0;}
body{background:#0e0e10;color:#ececef;font-family:-apple-system,sans-serif;padding:16px;line-height:1.4;}
h1{font-size:20px;margin-bottom:16px;}h2{font-size:14px;text-transform:uppercase;color:#9b9ba1;margin:20px 0 8px;letter-spacing:0.5px;}
.row{background:#1a1a1d;padding:10px 12px;border-radius:8px;margin-bottom:6px;display:flex;justify-content:space-between;align-items:center;}
.row .label{font-size:14px;color:#fff;}.row .val{font-size:14px;font-weight:600;color:#66bb6a;}
.bar{height:6px;background:#2a2a2d;border-radius:3px;overflow:hidden;margin-top:6px;}
.bar .fill{height:100%;background:#1565c0;}
</style></head><body>
<a href="/" style="display:inline-block;background:#1a1a1d;padding:6px 12px;border-radius:6px;color:#64b5f6;text-decoration:none;font-size:13px;margin-bottom:14px;">← 🏠 Accueil</a>
<h1>📊 Stats</h1>

<h2>Total par statut Hydracker</h2>
{{range .ByStatus}}
<div class="row"><div class="label">{{.Label}}</div><div class="val">{{.Count}}</div></div>
{{end}}

<h2>Top teams (jobs traités)</h2>
{{range .ByTeam}}
<div class="row"><div class="label">{{.Label}}</div><div class="val">{{.Count}}</div></div>
{{end}}

<h2>Par catégorie</h2>
{{range .ByCategory}}
<div class="row"><div class="label">{{.Label}}</div><div class="val">{{.Count}}</div></div>
{{end}}
</body></html>`

type statRow struct {
	Label string
	Count int
}

var adminStatsTplCompiled = template.Must(template.New("st").Parse(adminStatsTpl))

func renderAdminStats(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	q := func(sql string) []statRow {
		var out []statRow
		rows, err := db.Query(sql)
		if err != nil {
			return out
		}
		defer rows.Close()
		for rows.Next() {
			var s statRow
			if err := rows.Scan(&s.Label, &s.Count); err == nil {
				out = append(out, s)
			}
		}
		return out
	}
	data := map[string]any{
		"ByStatus":   q(`SELECT COALESCE(hydracker_status,'(en cours)'), COUNT(*) FROM jobs GROUP BY 1 ORDER BY 2 DESC`),
		"ByTeam":     q(`SELECT COALESCE(NULLIF(team,''),'(inconnu)'), COUNT(*) FROM jobs GROUP BY 1 ORDER BY 2 DESC LIMIT 15`),
		"ByCategory": q(`SELECT COALESCE(NULLIF(category,''),'(inconnu)'), COUNT(*) FROM jobs GROUP BY 1 ORDER BY 2 DESC LIMIT 15`),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminStatsTplCompiled.Execute(w, data)
}

// ---------- Logs page ----------

const adminLogsTpl = `<!doctype html>
<html lang="fr"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="refresh" content="15">
<title>Logs — Auto Post</title>
<style>
:root{color-scheme:dark;}*{box-sizing:border-box;margin:0;padding:0;}
body{background:#0e0e10;color:#ececef;font-family:-apple-system,sans-serif;padding:16px;}
h1{font-size:20px;margin-bottom:12px;}
pre{background:#1a1a1d;padding:12px;border-radius:8px;overflow-x:auto;font-family:'SF Mono',Monaco,Consolas,monospace;font-size:11px;line-height:1.4;white-space:pre-wrap;word-break:break-all;color:#cbd5e1;}
.refresh{color:#9b9ba1;font-size:10px;text-align:center;margin-top:12px;}
</style></head><body>
<a href="/" style="display:inline-block;background:#1a1a1d;padding:6px 12px;border-radius:6px;color:#64b5f6;text-decoration:none;font-size:13px;margin-bottom:14px;">← 🏠 Accueil</a>
<h1>📜 Logs auto-post (200 dernières lignes)</h1>
<pre>{{.Logs}}</pre>
<div class="refresh">↻ auto-refresh 15s</div>
</body></html>`

var adminLogsTplCompiled = template.Must(template.New("lg").Parse(adminLogsTpl))

func renderAdminLogs(w http.ResponseWriter, r *http.Request) {
	out, _ := exec.Command("journalctl", "-u", "auto-post.service", "-n", "200", "--no-pager", "--output=cat").Output()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminLogsTplCompiled.Execute(w, map[string]string{"Logs": string(out)})
}

// ---------- Seen items page ----------

const adminSeenTpl = `<!doctype html>
<html lang="fr"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Seen — Auto Post</title>
<style>
:root{color-scheme:dark;}*{box-sizing:border-box;margin:0;padding:0;}
body{background:#0e0e10;color:#ececef;font-family:-apple-system,sans-serif;padding:16px;line-height:1.4;}
h1{font-size:20px;margin-bottom:12px;}
.row{background:#1a1a1d;padding:8px 10px;border-radius:6px;margin-bottom:4px;font-size:12px;}
.row .title{color:#fff;font-weight:600;word-break:break-all;}
.row .meta{color:#9b9ba1;margin-top:2px;font-size:10px;}
.tag{display:inline-block;padding:1px 6px;border-radius:4px;font-size:9px;font-weight:600;margin-right:4px;background:#2a2a2d;}
form{margin-bottom:12px;background:#1a1a1d;padding:10px;border-radius:8px;}
input{width:100%;padding:10px;background:#0e0e10;border:1px solid #2a2a2d;border-radius:6px;color:#fff;font-size:14px;}
</style></head><body>
<a href="/" style="display:inline-block;background:#1a1a1d;padding:6px 12px;border-radius:6px;color:#64b5f6;text-decoration:none;font-size:13px;margin-bottom:14px;">← 🏠 Accueil</a>
<h1>👁 Seen items ({{.Total}})</h1>
<form method="GET" action="/admin/seen">
  <input name="q" value="{{.Query}}" placeholder="Filtrer par titre/team…" autofocus>
</form>
{{range .Items}}
<div class="row">
  <div class="title">{{.Title}}</div>
  <div class="meta">
    <span class="tag">{{.Category}}</span>
    {{if .Team}}<span class="tag">{{.Team}}</span>{{end}}
    · {{.SeenAt}}
  </div>
</div>
{{else}}
<div class="row" style="text-align:center;color:#9b9ba1;">Aucun item</div>
{{end}}
</body></html>`

type seenRow struct {
	Title    string
	Category string
	Team     string
	SeenAt   string
}

var adminSeenTplCompiled = template.Must(template.New("se").Parse(adminSeenTpl))

func renderAdminSeen(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var (
		total int
		rows  *sql.Rows
		err   error
	)
	if q != "" {
		_ = db.QueryRow(`SELECT COUNT(*) FROM seen_items WHERE title LIKE ? OR team LIKE ?`, "%"+q+"%", "%"+q+"%").Scan(&total)
		rows, err = db.Query(`SELECT title, category, team, seen_at FROM seen_items
			WHERE title LIKE ? OR team LIKE ?
			ORDER BY seen_at DESC LIMIT 200`, "%"+q+"%", "%"+q+"%")
	} else {
		_ = db.QueryRow(`SELECT COUNT(*) FROM seen_items`).Scan(&total)
		rows, err = db.Query(`SELECT title, category, team, seen_at FROM seen_items ORDER BY seen_at DESC LIMIT 200`)
	}
	var items []seenRow
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var s seenRow
			var ts int64
			if err := rows.Scan(&s.Title, &s.Category, &s.Team, &ts); err == nil {
				if ts > 0 {
					s.SeenAt = time.Unix(ts, 0).Format("01/02 15:04")
				}
				items = append(items, s)
			}
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminSeenTplCompiled.Execute(w, map[string]any{
		"Items": items,
		"Total": total,
		"Query": q,
	})
}

// ---------- Config page ----------

const adminConfigTpl = `<!doctype html>
<html lang="fr"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Config — Auto Post</title>
<style>
:root{color-scheme:dark;}*{box-sizing:border-box;margin:0;padding:0;}
body{background:#0e0e10;color:#ececef;font-family:-apple-system,sans-serif;padding:16px;}
h1{font-size:20px;margin-bottom:12px;}
pre{background:#1a1a1d;padding:14px;border-radius:8px;overflow-x:auto;font-family:'SF Mono',Monaco,Consolas,monospace;font-size:12px;line-height:1.5;white-space:pre-wrap;color:#cbd5e1;}
.note{color:#9b9ba1;font-size:11px;margin-bottom:12px;background:#1a1a1d;padding:10px;border-radius:8px;}
</style></head><body>
<a href="/" style="display:inline-block;background:#1a1a1d;padding:6px 12px;border-radius:6px;color:#64b5f6;text-decoration:none;font-size:13px;margin-bottom:14px;">← 🏠 Accueil</a>
<h1>⚙️ Config (read-only)</h1>
<div class="note">🔒 Les secrets (tokens, passwords, API keys) sont masqués. Pour modifier la config, édite <code>/etc/auto-post/config.yaml</code> sur le VPS et restart le service.</div>
<pre>{{.Config}}</pre>
</body></html>`

var adminConfigTplCompiled = template.Must(template.New("cf").Parse(adminConfigTpl))

func renderAdminConfig(w http.ResponseWriter, r *http.Request) {
	raw, err := os.ReadFile("/etc/auto-post/config.yaml")
	content := ""
	if err == nil {
		content = maskSecrets(string(raw))
	} else {
		content = "Erreur lecture: " + err.Error()
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminConfigTplCompiled.Execute(w, map[string]string{"Config": content})
}

// maskSecrets : remplace les valeurs sensibles par "REDACTED".
func maskSecrets(s string) string {
	patterns := []string{"token", "password", "api_key", "key", "secret", "bot_token", "chat_id"}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		low := strings.ToLower(line)
		for _, p := range patterns {
			if strings.Contains(low, p+":") || strings.Contains(low, p+" :") {
				if idx := strings.Index(line, ":"); idx > 0 {
					before := line[:idx+1]
					lines[i] = before + " \"REDACTED\""
				}
				break
			}
		}
		// URL avec ?key=...
		if strings.Contains(line, "key=") {
			if idx := strings.Index(line, "key="); idx > 0 {
				end := idx + 4
				for end < len(line) && line[end] != '"' && line[end] != '&' && line[end] != ' ' {
					end++
				}
				lines[i] = line[:idx+4] + "REDACTED" + line[end:]
			}
		}
	}
	return strings.Join(lines, "\n")
}

// ---------- helpers ----------

func splitTrimNonEmpty(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func atoiSafe(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return def
	}
	return n
}

const ackPageTpl = `<!doctype html>
<html><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>OK — {{.Action}}</title>
<style>
  body { background: #0e0e10; color: #ececef; font-family: -apple-system, sans-serif; padding: 32px; text-align: center; }
  h1 { font-size: 24px; margin-bottom: 12px; }
  p { color: #9b9ba1; margin-bottom: 24px; }
  a { color: #64b5f6; }
</style>
</head><body>
<h1>{{.Emoji}} {{.Action}}</h1>
<p>{{.Detail}}</p>
<a href="/" style="display:inline-block;background:#1a1a1d;padding:10px 18px;border-radius:8px;color:#64b5f6;text-decoration:none;font-size:14px;margin-top:8px;">← 🏠 Retour à l'accueil</a>
</body></html>
`

var (
	jobTplCompiled = template.Must(template.New("job").Parse(jobPageTpl))
	ackTplCompiled = template.Must(template.New("ack").Parse(ackPageTpl))
)

// renderJobPage : GET /jobs/{id} (HTML pour Safari iOS depuis tap notif)
func renderJobPage(cfg *Config, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/jobs/")
	jobID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "bad job id", http.StatusBadRequest)
		return
	}
	row := db.QueryRow(`SELECT title, COALESCE(tmdb_id,0), COALESCE(tmdb_title,''),
		COALESCE(tmdb_year,''), COALESCE(tmdb_poster,''), COALESCE(tmdb_score,0),
		COALESCE(tmdb_status,''), COALESCE(tmdb_alts_json,'[]')
		FROM jobs WHERE id=?`, jobID)
	var v jobView
	v.ID = jobID
	var altsJSON string
	err = row.Scan(&v.Filename, &v.BestID, &v.BestTitle, &v.BestYear, &v.BestPoster, &v.BestScore, &v.Status, &altsJSON)
	if err != nil {
		http.Error(w, "job introuvable", http.StatusNotFound)
		return
	}
	v.Token = cfg.Ntfy.WebhookSecret
	v.Release = v.Filename

	var alts []tmdb.Movie
	_ = json.Unmarshal([]byte(altsJSON), &alts)
	for i, a := range alts {
		v.Alts = append(v.Alts, altView{
			Idx:    i,
			ID:     a.ID,
			Title:  a.DisplayTitle(),
			Year:   a.Year(),
			Poster: a.PosterURL(),
			URL:    fmt.Sprintf("https://www.themoviedb.org/movie/%d", a.ID),
		})
	}

	// Top 3 acteurs (1 appel proxy TMDB, ~200ms)
	if v.BestID > 0 {
		v.Cast = fetchMovieCast(cfg.TMDB.ProxyURL, v.BestID, 3)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = jobTplCompiled.Execute(w, v)
}

// renderAckPage : page de confirmation après action POST
func renderAckPage(w http.ResponseWriter, emoji, action, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = ackTplCompiled.Execute(w, map[string]string{
		"Emoji":  emoji,
		"Action": action,
		"Detail": detail,
	})
}

// formAuthMiddleware : pour les POST depuis le navigateur, accepte le token soit
// en header X-Auto-Post-Token, soit en form field "token". Pour les GET, pas
// d'auth (rendu HTML basé sur des données déjà filtrées par Tailscale).
func formAuthMiddleware(cfg *Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GET pages publiques (Tailnet only via UFW) : home + jobs + admin GET
		if r.URL.Path == "/healthz" ||
			(r.Method == "GET" && r.URL.Path == "/") ||
			strings.HasPrefix(r.URL.Path, "/jobs/") ||
			(r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/admin/")) {
			next.ServeHTTP(w, r)
			return
		}
		if cfg.Ntfy.WebhookSecret == "" {
			next.ServeHTTP(w, r)
			return
		}
		// Header path (ntfy actions)
		if r.Header.Get("X-Auto-Post-Token") == cfg.Ntfy.WebhookSecret {
			next.ServeHTTP(w, r)
			return
		}
		// Form path (browser POST)
		if err := r.ParseForm(); err == nil {
			if r.Form.Get("token") == cfg.Ntfy.WebhookSecret {
				next.ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

// makeBrowserPostHandler : version qui rend une page de confirmation HTML
// au lieu d'un simple "ok" texte.
func makeBrowserPostHandler(cfg *Config, db *sql.DB, tmdbClient *tmdb.Client, action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
			title, year := getJobTMDBInfo(db, jobID)
			renderAckPage(w, "✅", "Confirmé", fmt.Sprintf("%s (%s)", title, year))
		case "skip":
			handleNtfySkip(cfg, db, jobID)
			renderAckPage(w, "❌", "Skipped", getJobTitle(db, jobID))
		case "alt":
			if len(parts) < 3 {
				http.Error(w, "alt index manquant", http.StatusBadRequest)
				return
			}
			altIdx, _ := strconv.Atoi(parts[2])
			handleNtfyAlt(cfg, db, jobID, altIdx)
			title, year := getJobTMDBInfo(db, jobID)
			renderAckPage(w, "✅", fmt.Sprintf("Alt %d confirmé", altIdx+1), fmt.Sprintf("%s (%s)", title, year))
		case "manual-form":
			// Form path : tmdb_id vient du form
			if err := r.ParseForm(); err != nil {
				http.Error(w, "form error", http.StatusBadRequest)
				return
			}
			tmdbID, err := strconv.Atoi(r.Form.Get("tmdb_id"))
			if err != nil || tmdbID <= 0 {
				http.Error(w, "tmdb_id invalide", http.StatusBadRequest)
				return
			}
			handleNtfyManual(cfg, db, tmdbClient, jobID, tmdbID)
			title, year := getJobTMDBInfo(db, jobID)
			renderAckPage(w, "✅", fmt.Sprintf("ID manuel %d", tmdbID), fmt.Sprintf("%s (%s)", title, year))
		}
	}
}
