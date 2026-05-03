package main

// Couche d'abstraction notifications : ntfy si configuré, Telegram sinon.
// Permet de switch sans toucher 30 call sites — chaque endroit appelle juste
// notify*() et le wrapper choisit le backend.

import (
	"database/sql"
	"fmt"
	"strings"
)

// useDiscord : true si Discord est configuré (priorité absolue)
func useDiscord(cfg *Config) bool {
	return cfg.Discord.Token != "" && cfg.Discord.ChannelID != ""
}

// useNtfy : true si ntfy URL est configuré (fallback si Discord pas configuré)
func useNtfy(cfg *Config) bool {
	return cfg.Ntfy.URL != ""
}

// notifySubmitted : NZB soumis à SAB (silencieux)
func notifySubmitted(cfg *Config, item RSSItem, team string) {
	if useDiscord(cfg) {
		// Submitted = silencieux, pas important — on logue juste, pas de notif Discord
		return
	}
	if useNtfy(cfg) {
		emoji := "📦"
		switch {
		case strings.HasPrefix(item.Category, "TV-"):
			emoji = "📺"
		case strings.HasPrefix(item.Category, "MOVIE-"):
			emoji = "🎬"
		}
		title := emoji + " NZB en téléchargement"
		msg := item.Title + "\n\nCat: " + item.Category
		if team != "" {
			msg += "\nTeam: " + team
		}
		_ = sendNtfy(cfg, title, msg, true)
		return
	}
	if cfg.Telegram.BotToken != "" {
		_ = sendTelegram(cfg.Telegram.BotToken, cfg.Telegram.ChatID, formatNotifSubmitted(item, team), true)
	}
}

// notifyCompleted : MKV prêt
func notifyCompleted(cfg *Config, title, mkvPath string) {
	if useDiscord(cfg) {
		// MKV ready = juste informatif, pas de notif (le post auto va suivre)
		return
	}
	if useNtfy(cfg) {
		_ = sendNtfy(cfg, "📥 MKV récupéré", title+"\n\n"+mkvPath, false)
		return
	}
	if cfg.Telegram.BotToken != "" {
		_ = sendTelegram(cfg.Telegram.BotToken, cfg.Telegram.ChatID, formatNotifCompleted(title, mkvPath), false)
	}
}

// notifyFailed : erreur générique
func notifyFailed(cfg *Config, title, errMsg string) {
	if useDiscord(cfg) {
		_ = sendDiscordSimple("❌ Échec : "+truncStr(title, 80), errMsg, 0xb71c1c)
		return
	}
	if useNtfy(cfg) {
		_ = sendNtfy(cfg, "❌ Échec : "+truncStr(title, 60), errMsg, false)
		return
	}
	if cfg.Telegram.BotToken != "" {
		_ = sendTelegram(cfg.Telegram.BotToken, cfg.Telegram.ChatID, formatNotifFailed(title, errMsg), false)
	}
}

// notifyText : message libre (utilisé pour récaps post Hydracker, etc.)
// htmlBody est utilisé pour Telegram (HTML escape déjà fait par l'appelant) ;
// pour ntfy, on strippe les tags HTML basiques.
func notifyText(cfg *Config, title, htmlBody string, silent bool) {
	if useDiscord(cfg) {
		color := 0x1e88e5
		if strings.Contains(title, "❌") {
			color = 0xb71c1c
		} else if strings.Contains(title, "⚠️") || strings.Contains(title, "⏭") {
			color = 0xed6c02
		} else if strings.Contains(title, "✅") {
			color = 0x2e7d32
		}
		_ = sendDiscordSimple(title, stripHTML(htmlBody), color)
		return
	}
	if useNtfy(cfg) {
		_ = sendNtfy(cfg, title, stripHTML(htmlBody), silent)
		return
	}
	if cfg.Telegram.BotToken != "" {
		_ = sendTelegram(cfg.Telegram.BotToken, cfg.Telegram.ChatID, htmlBody, silent)
	}
}

// notifyTMDBResult : verdict TMDB avec poster + boutons. Backend-aware.
// Retourne le telegram message_id si applicable (0 sinon).
func notifyTMDBResult(cfg *Config, db *sql.DB, jobID int64, release string, res TMDBResult) int {
	if useDiscord(cfg) {
		_, _ = sendDiscordTMDB(jobID, release, res)
		return 0
	}
	if useNtfy(cfg) {
		title, msg, silent := formatTMDBNtfyContent(release, res)
		actions := buildTMDBNtfyActions(cfg, jobID, res)
		var posterURL string
		if res.Best != nil {
			posterURL = res.Best.PosterURL()
		}
		// Click URL = mini page web sur le VPS (toutes les alts + posters + manual ID)
		clickURL := fmt.Sprintf("%s/jobs/%d", strings.TrimRight(cfg.Ntfy.WebhookBaseURL, "/"), jobID)
		_ = sendNtfyWithActions(cfg, title, msg, posterURL, clickURL, actions, silent)
		return 0
	}
	if cfg.Telegram.BotToken != "" {
		caption, silent := formatTMDBNotif(release, res)
		buttons := buildTMDBButtons(jobID, res)
		var posterURL string
		if res.Best != nil {
			posterURL = res.Best.PosterURL()
		}
		msgID, _ := sendTelegramPhotoWithButtons(cfg.Telegram.BotToken, cfg.Telegram.ChatID, posterURL, caption, buttons, silent)
		return msgID
	}
	return 0
}

// stripHTML : enlève les tags HTML basiques (<b>, <i>, <code>, <a>) pour ntfy.
// Utile car les appelants ont des messages formatés HTML pour Telegram.
func stripHTML(s string) string {
	r := strings.NewReplacer(
		"<b>", "", "</b>", "",
		"<i>", "", "</i>", "",
		"<code>", "", "</code>", "",
		"&lt;", "<", "&gt;", ">", "&amp;", "&",
	)
	s = r.Replace(s)
	// Strip <a href="...">...</a>
	for {
		i := strings.Index(s, "<a ")
		if i < 0 {
			break
		}
		j := strings.Index(s[i:], ">")
		if j < 0 {
			break
		}
		s = s[:i] + s[i+j+1:]
	}
	s = strings.ReplaceAll(s, "</a>", "")
	return s
}
