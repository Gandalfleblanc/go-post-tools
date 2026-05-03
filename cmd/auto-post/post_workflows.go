package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go-post-tools/api"
	"go-post-tools/internal/ftpup"
	"go-post-tools/internal/parser"
	"go-post-tools/internal/seedbox"
	"go-post-tools/internal/torrent"
	"go-post-tools/internal/uploader"
)

// mediaInfoJSON : sortie de mediainfo --Output=JSON
type mediaInfoTrack struct {
	Type             string `json:"@type"`
	FileSize         string `json:"FileSize"`
	Duration         string `json:"Duration"`
	OverallBitRate   string `json:"OverallBitRate"`
	Format           string `json:"Format"`
	FormatProfile    string `json:"Format_Profile"`
	CodecID          string `json:"CodecID"`
	Width            string `json:"Width"`
	Height           string `json:"Height"`
	BitRate          string `json:"BitRate"`
	FrameRate        string `json:"FrameRate"`
	HDRFormat        string `json:"HDR_Format"`
	Channels         string `json:"Channels"`
	ChannelLayout    string `json:"ChannelLayout"`
	Language         string `json:"Language"`
	Title            string `json:"Title"`
	Default          string `json:"Default"`
	Forced           string `json:"Forced"`
	SamplingRate     string `json:"SamplingRate"`
}

type mediaInfoJSON struct {
	Media struct {
		Tracks []mediaInfoTrack `json:"track"`
	} `json:"media"`
}

// getMediaInfoNFO : génère un NFO lisible/aéré depuis mediainfo JSON.
// Si erreur ou binaire absent, retourne "" (post sans NFO, pas critique).
func getMediaInfoNFO(mkvPath string) string {
	return buildNiceNFO(mkvPath, 0)
}

// buildNiceNFO : tmdbID optionnel pour ajouter le lien TMDB dans le NFO.
func buildNiceNFO(mkvPath string, tmdbID int) string {
	if _, err := os.Stat(mkvPath); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "mediainfo", "--Output=JSON", mkvPath).Output()
	if err != nil {
		log.Printf("[mediainfo] %s: %v", filepath.Base(mkvPath), err)
		return ""
	}
	var mi mediaInfoJSON
	if err := json.Unmarshal(out, &mi); err != nil {
		log.Printf("[mediainfo] parse JSON: %v", err)
		return ""
	}

	var general mediaInfoTrack
	var video mediaInfoTrack
	var audios []mediaInfoTrack
	var subs []mediaInfoTrack
	for _, t := range mi.Media.Tracks {
		switch t.Type {
		case "General":
			general = t
		case "Video":
			if video.Type == "" {
				video = t
			}
		case "Audio":
			audios = append(audios, t)
		case "Text":
			subs = append(subs, t)
		}
	}

	bar := strings.Repeat("─", 64)
	var b strings.Builder
	fmt.Fprintln(&b, bar)
	fmt.Fprintln(&b, "  📦 "+strings.TrimSuffix(filepath.Base(mkvPath), filepath.Ext(mkvPath)))
	fmt.Fprintln(&b, bar)
	fmt.Fprintln(&b)

	// FICHIER
	fmt.Fprintln(&b, "  ▶ FICHIER")
	if v := humanBytes(general.FileSize); v != "" {
		fmt.Fprintf(&b, "    Taille     : %s\n", v)
	}
	if v := humanDuration(general.Duration); v != "" {
		fmt.Fprintf(&b, "    Durée      : %s\n", v)
	}
	if v := humanBitrate(general.OverallBitRate); v != "" {
		fmt.Fprintf(&b, "    Bitrate    : %s\n", v)
	}
	fmt.Fprintln(&b)

	// VIDEO
	if video.Type != "" {
		fmt.Fprintln(&b, "  ▶ VIDÉO")
		if v := video.Format; v != "" {
			profile := ""
			if video.FormatProfile != "" {
				profile = " (" + video.FormatProfile + ")"
			}
			fmt.Fprintf(&b, "    Codec      : %s%s\n", v, profile)
		}
		if video.Width != "" && video.Height != "" {
			res := classifyRes(video.Width, video.Height)
			fmt.Fprintf(&b, "    Résolution : %s × %s%s\n", video.Width, video.Height, res)
		}
		if v := humanBitrate(video.BitRate); v != "" {
			fmt.Fprintf(&b, "    Bitrate    : %s\n", v)
		}
		if v := video.FrameRate; v != "" {
			fmt.Fprintf(&b, "    Framerate  : %s fps\n", v)
		}
		if v := video.HDRFormat; v != "" {
			fmt.Fprintf(&b, "    HDR        : %s\n", v)
		}
		fmt.Fprintln(&b)
	}

	// AUDIO
	if len(audios) > 0 {
		fmt.Fprintln(&b, "  ▶ AUDIO")
		for i, a := range audios {
			lang := strings.ToUpper(a.Language)
			if lang == "" {
				lang = "?"
			}
			ch := a.ChannelLayout
			if ch == "" && a.Channels != "" {
				ch = a.Channels + "ch"
			}
			fmt.Fprintf(&b, "    [%d] %-6s · %s · %s",
				i+1, lang, a.Format, ch)
			if v := humanBitrate(a.BitRate); v != "" {
				fmt.Fprintf(&b, " · %s", v)
			}
			if a.Title != "" {
				fmt.Fprintf(&b, " · %s", a.Title)
			}
			fmt.Fprintln(&b)
		}
		fmt.Fprintln(&b)
	}

	// SOUS-TITRES
	if len(subs) > 0 {
		fmt.Fprintln(&b, "  ▶ SOUS-TITRES")
		for i, s := range subs {
			lang := strings.ToUpper(s.Language)
			if lang == "" {
				lang = "?"
			}
			extra := ""
			if s.Forced == "Yes" {
				extra = " (forced)"
			}
			if s.Title != "" {
				extra += " — " + s.Title
			}
			fmt.Fprintf(&b, "    [%d] %s · %s%s\n", i+1, lang, s.Format, extra)
		}
		fmt.Fprintln(&b)
	}

	// TMDB
	if tmdbID > 0 {
		fmt.Fprintln(&b, "  ▶ SOURCE")
		fmt.Fprintf(&b, "    TMDB       : https://www.themoviedb.org/movie/%d\n", tmdbID)
		fmt.Fprintln(&b)
	}

	fmt.Fprintln(&b, bar)
	fmt.Fprintln(&b, "                       Posted via Auto Post")
	fmt.Fprintln(&b, bar)
	return b.String()
}

// humanBytes : "5826123456" → "5.42 GiB"
func humanBytes(s string) string {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return ""
	}
	const (
		ki = 1024
		mi = 1024 * 1024
		gi = 1024 * 1024 * 1024
	)
	switch {
	case n >= gi:
		return fmt.Sprintf("%.2f GiB", float64(n)/float64(gi))
	case n >= mi:
		return fmt.Sprintf("%.2f MiB", float64(n)/float64(mi))
	case n >= ki:
		return fmt.Sprintf("%.2f KiB", float64(n)/float64(ki))
	}
	return fmt.Sprintf("%d B", n)
}

// humanDuration : "5520.000" (secondes) → "1 h 32 min"
func humanDuration(s string) string {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return ""
	}
	total := int(f)
	h := total / 3600
	m := (total % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%d h %02d min", h, m)
	}
	return fmt.Sprintf("%d min", m)
}

// humanBitrate : "8530000" → "8.53 Mb/s"
func humanBitrate(s string) string {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return ""
	}
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.2f Mb/s", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%d kb/s", n/1_000)
	}
	return fmt.Sprintf("%d b/s", n)
}

// classifyRes : "(4K)" / "(1080p)" / etc.
func classifyRes(w, h string) string {
	hi, _ := strconv.Atoi(h)
	switch {
	case hi >= 2000:
		return " (2160p / 4K)"
	case hi >= 1000:
		return " (1080p)"
	case hi >= 700:
		return " (720p)"
	case hi >= 400:
		return " (SD)"
	}
	return ""
}

// ---------- Hydracker meta cache ----------

type hydrackerMeta struct {
	// langs : nom canonique → name à passer dans langues[]
	// (ex: "french" → "French", "truefrench" → "TrueFrench")
	langs map[string]string
	// qualities : key normalisée ("1080p", "4k", "2160p") → ID Hydracker
	qualities map[string]int
}

func loadHydrackerMeta(hyd *api.Client) (*hydrackerMeta, error) {
	m := &hydrackerMeta{
		langs:     make(map[string]string),
		qualities: make(map[string]int),
	}
	langs, err := hyd.GetLangs()
	if err != nil {
		return nil, fmt.Errorf("get langs: %w", err)
	}
	for _, l := range langs {
		m.langs[strings.ToLower(l.Name)] = l.Name
	}
	quals, err := hyd.GetQualities()
	if err != nil {
		return nil, fmt.Errorf("get quals: %w", err)
	}
	for _, q := range quals {
		key := strings.ToLower(q.Name)
		m.qualities[key] = q.ID
		// Alias 4K ↔ 2160p
		if key == "4k" {
			m.qualities["2160p"] = q.ID
		}
		if key == "2160p" {
			m.qualities["4k"] = q.ID
		}
	}
	return m, nil
}

// detectQuality : mappe résolution + source + codec → ID qualité Hydracker.
// Hydracker classe par format (WEB 1080p, BluRay 1080p (x265), ULTRA HD (x265), etc.)
// et pas juste par résolution.
func detectQuality(meta *hydrackerMeta, info *parser.FileInfo) int {
	res := strings.ToLower(info.Quality)     // "1080p", "4k", "720p"
	src := strings.ToLower(info.Source)      // "bluray","bdrip","web-dl","webrip","hdlight","hdtv","dvdrip","bdremux"
	codec := strings.ToLower(info.VideoCodec) // "h.265","h.264","hevc"
	isHEVC := strings.Contains(codec, "265") || strings.Contains(codec, "hevc")

	// Fallback : si parser n'a pas reconnu la source (cas ".WEB." seul, "MULTi.WEB.H264"),
	// déduit depuis le titre brut.
	if src == "" {
		lt := strings.ToLower(info.Raw)
		switch {
		case strings.Contains(lt, "hdlight"):
			src = "hdlight"
		case strings.Contains(lt, "remux"):
			src = "bdremux"
		case strings.Contains(lt, "web"):
			// Couvre WEB-DL, WEBDL, WEBRip, .WEB., -WEB-, .WEB (fin de chaîne après strip ext)
			src = "web"
		case strings.Contains(lt, "bluray") || strings.Contains(lt, "blu-ray") || strings.Contains(lt, "bdrip"):
			src = "bluray"
		case strings.Contains(lt, "hdtv"):
			src = "hdtv"
		case strings.Contains(lt, "dvdrip"):
			src = "dvdrip"
		}
	}

	find := func(name string) int {
		if id, ok := meta.qualities[strings.ToLower(name)]; ok {
			return id
		}
		return 0
	}

	// 4K / 2160p — toujours x265 chez Hydracker.
	// Ordre de priorité : HDLight (4K Light) > Remux > Ultra HD générique.
	if res == "4k" || res == "2160p" {
		if strings.Contains(src, "hdlight") {
			if id := find("ultra hdlight (x265)"); id > 0 {
				return id
			}
		}
		if strings.Contains(src, "remux") {
			if id := find("remux uhd"); id > 0 {
				return id
			}
		}
		if id := find("ultra hd (x265)"); id > 0 {
			return id
		}
	}

	// 1080p
	if res == "1080p" {
		isRemux := strings.Contains(src, "remux") || strings.Contains(src, "bdremux")
		isBluRay := strings.Contains(src, "blu") || src == "bdrip"
		isWeb := strings.Contains(src, "web")
		isHDLight := strings.Contains(src, "hdlight")
		isHDTV := strings.Contains(src, "hdtv")
		switch {
		case isRemux:
			return find("remux bluray")
		case isBluRay:
			if isHEVC {
				return find("blu-ray 1080p (x265)")
			}
			return find("blu-ray 1080p")
		case isWeb:
			if isHEVC {
				return find("web 1080p (x265)")
			}
			return find("web 1080p")
		case isHDLight:
			if isHEVC {
				return find("hdlight 1080p (x265)")
			}
			return find("hdlight 1080p")
		case isHDTV:
			return find("hdtv 1080p")
		default:
			return find("hd 1080p")
		}
	}

	// 720p
	if res == "720p" {
		switch {
		case strings.Contains(src, "blu") || src == "bdrip":
			return find("blu-ray 720p")
		case strings.Contains(src, "web"):
			return find("web 720p")
		case strings.Contains(src, "hdlight"):
			return find("hdlight 720p")
		case strings.Contains(src, "hdtv"):
			return find("hdtv 720p")
		default:
			return find("hd 720p")
		}
	}

	// SD fallbacks
	if strings.Contains(src, "dvdrip") {
		return find("dvdrip")
	}
	if src == "bdrip" {
		return find("bdrip")
	}
	if strings.Contains(src, "web") {
		return find("web")
	}
	if strings.Contains(src, "hdtv") {
		return find("hdtv")
	}
	if strings.Contains(src, "hdrip") {
		return find("hdrip")
	}
	return 0
}

// detectLanguages : retourne les langues canoniques pour Hydracker.
// MULTi → ["TrueFrench", "English"] (convention scène française)
// French/VF/VFF → ["French"]
// TRUEFRENCH → ["TrueFrench"]
// Si rien détecté → ["TrueFrench"] (films français par défaut)
func detectLanguages(meta *hydrackerMeta, info *parser.FileInfo) []string {
	var out []string
	seen := map[string]bool{}
	add := func(name string) {
		canonical, ok := meta.langs[strings.ToLower(name)]
		if !ok {
			return
		}
		if !seen[canonical] {
			seen[canonical] = true
			out = append(out, canonical)
		}
	}
	for _, l := range info.Languages {
		switch strings.ToLower(l) {
		case "multi":
			add("TrueFrench")
			add("English")
		default:
			add(l)
		}
	}
	if len(out) == 0 {
		add("TrueFrench")
	}
	return out
}

// ---------- Hydracker helpers ----------

// findHydrackerTitle : GET /titles?tmdb_id=X. Retourne (titleID, found, err).
func findHydrackerTitle(hyd *api.Client, tmdbID int) (int, bool, error) {
	pt, err := hyd.GetTitleByTmdbID(tmdbID)
	if err != nil {
		return 0, false, err
	}
	if pt == nil || pt.ID == 0 {
		return 0, false, nil
	}
	return pt.ID, true, nil
}

// downloadHydrackerTorrent : récupère le .torrent généré par Hydracker (avec passkey)
// via GET /torrents/{id}/download. L'API client ne l'expose pas, donc HTTP direct.
func downloadHydrackerTorrentRaw(baseURL, token string, torrentID int) ([]byte, error) {
	apiURL := fmt.Sprintf("%s/torrents/%d/download", strings.TrimRight(baseURL, "/"), torrentID)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/x-bittorrent")
	req.Header.Set("User-Agent", "go-post-tools-auto-post/1.0")
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
		return nil, fmt.Errorf("réponse n'est pas un .torrent valide (Content-Type: %s)", resp.Header.Get("Content-Type"))
	}
	return data, nil
}

// ---------- Workflows ----------

// runDDLWorkflow : upload sur 1Fichier puis Send.now en série, post lien sur Hydracker.
// Retourne les URLs uploadées + erreur si critique.
func runDDLWorkflow(cfg *Config, hyd *api.Client, mkvPath string, titleID, qualite int, langues []string, nfo string) ([]string, error) {
	ctx := context.Background()
	var urls []string

	if cfg.OneFichier.APIKey != "" {
		log.Printf("[auto-post] DDL upload 1Fichier (%s)...", filepath.Base(mkvPath))
		res, err := uploader.UploadOneFichier(ctx, cfg.OneFichier.APIKey, mkvPath, nil)
		if err != nil {
			return urls, fmt.Errorf("1Fichier upload: %w", err)
		}
		if res != nil && res.URL != "" {
			urls = append(urls, res.URL)
			log.Printf("[auto-post] 1Fichier OK: %s", res.URL)
			if err := withRetry("UploadLien-1F", 3, func() error {
				_, e := hyd.UploadLien(titleID, qualite, langues, nil, res.URL, nfo, 0, 0, false)
				return e
			}); err != nil {
				return urls, fmt.Errorf("Hydracker UploadLien (1Fichier): %w", err)
			}
		}
	}

	if cfg.SendCm.APIKey != "" {
		log.Printf("[auto-post] DDL upload Send.now (%s)...", filepath.Base(mkvPath))
		res, err := uploader.UploadSendCm(ctx, cfg.SendCm.APIKey, mkvPath, nil)
		if err != nil {
			return urls, fmt.Errorf("Send.now upload: %w", err)
		}
		if res != nil && res.URL != "" {
			urls = append(urls, res.URL)
			log.Printf("[auto-post] Send.now OK: %s", res.URL)
			if err := withRetry("UploadLien-Send", 3, func() error {
				_, e := hyd.UploadLien(titleID, qualite, langues, nil, res.URL, nfo, 0, 0, false)
				return e
			}); err != nil {
				return urls, fmt.Errorf("Hydracker UploadLien (Send.now): %w", err)
			}
		}
	}

	return urls, nil
}

// runNzbWorkflow : télécharge le .nzb original depuis l'URL unfr.pw,
// puis le poste sur Hydracker via UploadNzb. Retourne l'ID Hydracker du nzb.
func runNzbWorkflow(cfg *Config, hyd *api.Client, nzbURL string, titleID, qualite int, langues []string, nfo string) (int, error) {
	// 1. DL le .nzb dans /tmp
	tmpFile, err := os.CreateTemp("", "auto-post-*.nzb")
	if err != nil {
		return 0, fmt.Errorf("temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	req, err := http.NewRequest("GET", nzbURL, nil)
	if err != nil {
		tmpFile.Close()
		return 0, err
	}
	req.Header.Set("User-Agent", "go-post-tools-auto-post/1.0")
	c := &http.Client{Timeout: 60 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		tmpFile.Close()
		return 0, fmt.Errorf("DL nzb: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		tmpFile.Close()
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("DL nzb HTTP %d: %s", resp.StatusCode, string(body[:min(200, len(body))]))
	}
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return 0, err
	}
	tmpFile.Close()

	// Sanity check : un .nzb commence par <?xml ou <nzb
	info, _ := os.Stat(tmpPath)
	if info.Size() < 100 {
		return 0, fmt.Errorf(".nzb trop petit (%d bytes)", info.Size())
	}
	log.Printf("[nzb] DL OK %d bytes → upload Hydracker", info.Size())

	// 2. Upload sur Hydracker (avec retry sur 522/timeout)
	var res *api.UploadNzbResult
	if rerr := withRetry("UploadNzb", 3, func() error {
		var e error
		res, e = hyd.UploadNzb(titleID, qualite, langues, nil, tmpPath, nfo, 0, 0, false)
		return e
	}); rerr != nil {
		return 0, fmt.Errorf("Hydracker UploadNzb: %w", rerr)
	}
	if !res.Success {
		return 0, fmt.Errorf("Hydracker rejected NZB: %s", res.Message)
	}
	log.Printf("[nzb] Hydracker NZB ID = %d", res.Nzb.ID)
	return res.Nzb.ID, nil
}

// runTorrentWorkflow : FTP MKV → torrent.Create → Hydracker UploadTorrent →
// download .torrent modifié (avec passkey) → push vers ruTorrent.
func runTorrentWorkflow(cfg *Config, hyd *api.Client, mkvPath string, titleID, qualite int, langues []string, nfo string) (int, error) {
	ctx := context.Background()

	// 1. FTP upload du MKV
	log.Printf("[auto-post] FTP upload %s → %s:%d", filepath.Base(mkvPath), cfg.FTP.Host, cfg.FTP.Port)
	_, err := ftpup.Upload(ctx, cfg.FTP.Host, cfg.FTP.Port, cfg.FTP.User, cfg.FTP.Password, cfg.FTP.Path, mkvPath, nil)
	if err != nil {
		return 0, fmt.Errorf("FTP upload: %w", err)
	}

	// 2. Création du .torrent local
	torrentPath := mkvPath + ".torrent"
	pieceSize := cfg.Torrent.PieceSize
	if pieceSize == 0 {
		pieceSize = 8 * 1024 * 1024
	}
	log.Printf("[auto-post] torrent.Create %s (piece=%d)", filepath.Base(torrentPath), pieceSize)
	if err := torrent.Create(mkvPath, cfg.Torrent.TrackerURL, torrentPath, pieceSize, nil); err != nil {
		return 0, fmt.Errorf("torrent create: %w", err)
	}
	defer os.Remove(torrentPath)

	// 3. Upload sur Hydracker
	log.Printf("[auto-post] Hydracker UploadTorrent...")
	res, err := hyd.UploadTorrent(titleID, qualite, langues, nil, torrentPath, nfo, 0, 0, false)
	if err != nil {
		return 0, fmt.Errorf("Hydracker UploadTorrent: %w", err)
	}
	if !res.Success {
		return 0, fmt.Errorf("Hydracker rejected: %s", res.Message)
	}
	hydID := res.Torrent.ID
	log.Printf("[auto-post] Hydracker torrent ID = %d", hydID)

	// 4. Download du .torrent modifié (avec passkey)
	log.Printf("[auto-post] Download Hydracker .torrent...")
	modifiedTorrent, err := downloadHydrackerTorrentRaw(cfg.Hydracker.BaseURL, cfg.Hydracker.Token, hydID)
	if err != nil {
		return hydID, fmt.Errorf("download Hydracker torrent: %w", err)
	}
	modifiedPath := torrentPath + ".hyd"
	if err := os.WriteFile(modifiedPath, modifiedTorrent, 0644); err != nil {
		return hydID, fmt.Errorf("write modified torrent: %w", err)
	}
	defer os.Remove(modifiedPath)

	// 5. Push vers ruTorrent seedbox
	log.Printf("[auto-post] Push vers ruTorrent...")
	_, err = seedbox.Upload(ctx, cfg.Seedbox.URL, cfg.Seedbox.User, cfg.Seedbox.Password, "", modifiedPath, nil)
	if err != nil {
		return hydID, fmt.Errorf("seedbox upload: %w", err)
	}
	log.Printf("[auto-post] Seedbox OK")
	return hydID, nil
}

// ---------- Pipeline orchestrator ----------

// processConfirmedJob : pour un job confirmed, fait DDL puis NZB et update DB.
// (Le workflow torrent est désactivé pour le moment — uniquement NZB + DDL.)
func processConfirmedJob(cfg *Config, db *sql.DB, hyd *api.Client, meta *hydrackerMeta, jobID int64) {
	var (
		title, mkvPath, nzbURL string
		tmdbID                 int
	)
	err := db.QueryRow("SELECT title, mkv_path, nzb_url, tmdb_id FROM jobs WHERE id=?", jobID).Scan(&title, &mkvPath, &nzbURL, &tmdbID)
	if err != nil {
		log.Printf("[auto-post] processConfirmedJob: load job err: %v", err)
		return
	}
	now := time.Now().Unix()

	// 1. Trouve le title_id Hydracker
	hydTitleID, found, err := findHydrackerTitle(hyd, tmdbID)
	if err != nil {
		_, _ = db.Exec("UPDATE jobs SET hydracker_status='lookup_failed', hydracker_error=?, hydracker_processed_at=? WHERE id=?",
			err.Error(), now, jobID)
		log.Printf("[auto-post] Hydracker title lookup err: %v", err)
		notifyFailed(cfg, "Lookup Hydracker échoué : "+title, err.Error())
		return
	}
	if !found {
		_, _ = db.Exec("UPDATE jobs SET hydracker_status='no_title', hydracker_processed_at=? WHERE id=?",
			now, jobID)
		log.Printf("[auto-post] Aucune fiche Hydracker pour TMDB %d", tmdbID)
		notifyText(cfg, "❓ Pas de fiche Hydracker",
			fmt.Sprintf("%s\n\nCrée la fiche pour TMDB %d puis rerun (chunk #6 ajoutera le retry auto).", title, tmdbID), false)
		return
	}
	log.Printf("[auto-post] Hydracker title_id=%d pour TMDB %d", hydTitleID, tmdbID)

	// 2. Détecte qualité + langues
	info := parser.ParseFilename(title)
	qualite := detectQuality(meta, info)
	if qualite == 0 {
		_, _ = db.Exec("UPDATE jobs SET hydracker_status='unknown_quality', hydracker_processed_at=? WHERE id=?",
			now, jobID)
		notifyFailed(cfg, "Qualité inconnue : "+title, "Qualité parsée: "+info.Quality+" non mappée Hydracker.")
		log.Printf("[auto-post] Qualité inconnue: %s", info.Quality)
		removeMKVAndMark(db, jobID, mkvPath)
		return
	}
	langues := detectLanguages(meta, info)
	log.Printf("[auto-post] qualite=%d langues=%v", qualite, langues)

	// 2.5 Whitelist Hydracker quality : si la qualité parsée n'est pas dans
	// la liste autorisée par l'user → skip (= film hors champ).
	if !qualityAllowed(cfg, qualite) {
		_, _ = db.Exec("UPDATE jobs SET hydracker_status='quality_skipped', hydracker_processed_at=? WHERE id=?",
			now, jobID)
		log.Printf("[auto-post] Qualité %d non whitelistée (autorisées=%v) → skip", qualite, cfg.Filters.AllowedHydrackerQualities)
		notifyText(cfg, "⏭ Qualité non autorisée",
			fmt.Sprintf("%s\nQualité Hydracker #%d ignorée (hors whitelist)", title, qualite), true)
		removeMKVAndMark(db, jobID, mkvPath)
		return
	}

	// 2.6 Dédup Hydracker : si un NZB ET un lien DDL existent déjà pour cette
	// fiche+qualité → on ne re-poste rien (skip total). Si un seul des deux
	// manque → on poste seulement celui qui manque.
	nzbDup := false
	liensDup := false
	if r, err := hyd.GetNzbs(hydTitleID, api.ContentFilter{Quality: qualite}); err == nil && r.Count > 0 {
		nzbDup = true
	}
	if r, err := hyd.GetLiens(hydTitleID, api.ContentFilter{Quality: qualite}); err == nil && r.Count > 0 {
		liensDup = true
	}
	if nzbDup && liensDup {
		_, _ = db.Exec("UPDATE jobs SET hydracker_status='dup_skipped', hydracker_processed_at=? WHERE id=?",
			now, jobID)
		log.Printf("[auto-post] DUP Hydracker (NZB+DDL) déjà présents → skip")
		notifyText(cfg, "⏭ Doublon Hydracker",
			fmt.Sprintf("%s\nNZB+DDL déjà présents en qualité #%d", title, qualite), true)
		removeMKVAndMark(db, jobID, mkvPath)
		return
	}

	// 3. Update DB pour marquer "in progress"
	_, _ = db.Exec("UPDATE jobs SET hydracker_status='posting', hydracker_title_id=?, hydracker_processed_at=? WHERE id=?",
		hydTitleID, now, jobID)

	// 3.5 Génère le NFO (mediainfo formatté) une fois, partagé DDL + NZB
	nfo := buildNiceNFO(mkvPath, tmdbID)
	if nfo != "" {
		log.Printf("[auto-post] NFO généré (%d bytes)", len(nfo))
	}

	// 4. DDL workflow (sauf si déjà présent)
	var ddlURLs []string
	var ddlErr error
	if liensDup {
		log.Printf("[auto-post] DDL skip (déjà présent sur Hydracker)")
	} else {
		ddlURLs, ddlErr = runDDLWorkflow(cfg, hyd, mkvPath, hydTitleID, qualite, langues, nfo)
		if ddlErr != nil {
			log.Printf("[auto-post] DDL workflow err: %v", ddlErr)
		}
	}
	ddlURLsJSON, _ := json.Marshal(ddlURLs)

	// 5. NZB workflow (sauf si déjà présent)
	var nzbID int
	var nzbErr error
	if nzbDup {
		log.Printf("[auto-post] NZB skip (déjà présent sur Hydracker)")
	} else {
		nzbID, nzbErr = runNzbWorkflow(cfg, hyd, nzbURL, hydTitleID, qualite, langues, nfo)
		if nzbErr != nil {
			log.Printf("[auto-post] NZB workflow err: %v", nzbErr)
		}
	}

	// 6. Update final
	finalStatus := "posted"
	var errParts []string
	if ddlErr != nil {
		errParts = append(errParts, "DDL: "+ddlErr.Error())
	}
	if nzbErr != nil {
		errParts = append(errParts, "NZB: "+nzbErr.Error())
	}
	hadAttempts := !liensDup || !nzbDup
	if hadAttempts {
		if ddlErr != nil && nzbErr != nil {
			finalStatus = "failed"
		} else if ddlErr != nil || nzbErr != nil {
			finalStatus = "partial"
		}
	}

	_, _ = db.Exec(`UPDATE jobs SET
		hydracker_status=?, hydracker_nzb_id=?, hydracker_ddl_urls_json=?,
		hydracker_error=?, hydracker_processed_at=?
		WHERE id=?`,
		finalStatus, nzbID, string(ddlURLsJSON), strings.Join(errParts, " | "), now, jobID)

	// 7. Notif récap (toujours non-silencieuse, l'user veut être prévenu sur chaque post)
	caption := buildPostRecapNotif(title, finalStatus, ddlURLs, nzbID, errParts)
	emoji := "✅"
	switch finalStatus {
	case "failed":
		emoji = "❌"
	case "partial":
		emoji = "⚠️"
	}
	notifyText(cfg, emoji+" Post "+finalStatus+" : "+title, caption, false)
	log.Printf("[auto-post] POST %s for job=%d (%s)", finalStatus, jobID, title)

	// 8. Cleanup MKV immédiatement (l'user veut libérer le disque tout de suite)
	if finalStatus != "failed" {
		removeMKVAndMark(db, jobID, mkvPath)
	}
}

// qualityAllowed : true si pas de whitelist, OU si qualité dans la whitelist.
func qualityAllowed(cfg *Config, qualite int) bool {
	if len(cfg.Filters.AllowedHydrackerQualities) == 0 {
		return true
	}
	for _, q := range cfg.Filters.AllowedHydrackerQualities {
		if q == qualite {
			return true
		}
	}
	return false
}

// removeMKVAndMark : supprime le MKV (+ dossier SAB) et marque le cleanup_done_at.
func removeMKVAndMark(db *sql.DB, jobID int64, mkvPath string) {
	if mkvPath == "" {
		return
	}
	if err := removeJobFiles(mkvPath); err != nil {
		log.Printf("[auto-post] cleanup err job=%d: %v", jobID, err)
	} else {
		log.Printf("[auto-post] cleanup OK job=%d", jobID)
	}
	_, _ = db.Exec("UPDATE jobs SET cleanup_done_at=? WHERE id=?", time.Now().Unix(), jobID)
}

func buildPostRecapNotif(title, status string, ddlURLs []string, nzbID int, errs []string) string {
	emoji := "✅"
	statusLine := "<b>Posté</b>"
	switch status {
	case "failed":
		emoji = "❌"
		statusLine = "<b>Échec total</b>"
	case "partial":
		emoji = "⚠️"
		statusLine = "<b>Partiellement posté</b>"
	}
	out := fmt.Sprintf("%s %s\n\n<b>%s</b>\n\n", emoji, statusLine, htmlEscape(title))
	if len(ddlURLs) > 0 {
		out += "<b>📥 DDL :</b>\n"
		for _, u := range ddlURLs {
			out += "• <code>" + htmlEscape(u) + "</code>\n"
		}
		out += "\n"
	}
	if nzbID > 0 {
		out += fmt.Sprintf("<b>📦 NZB Hydracker :</b> <code>#%d</code>\n", nzbID)
	}
	if len(errs) > 0 {
		out += "\n<b>⚠️ Erreurs :</b>\n"
		for _, e := range errs {
			out += "• " + htmlEscape(e) + "\n"
		}
	}
	return out
}

// pollConfirmedJobs : trouve les jobs status='ready' AND tmdb_status='confirmed' AND
// hydracker_status NULL (pas encore traités) et lance le post pour chacun.
func pollConfirmedJobs(cfg *Config, db *sql.DB, hyd *api.Client, meta *hydrackerMeta) error {
	if isPaused() {
		return nil
	}
	rows, err := db.Query(`SELECT id FROM jobs
		WHERE status='ready' AND tmdb_status='confirmed' AND hydracker_status IS NULL
		ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()

	for _, id := range ids {
		processConfirmedJob(cfg, db, hyd, meta, id)
	}
	return nil
}
