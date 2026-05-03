package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go-post-tools/internal/tmdb"
)

// ---------- Telegram types ----------

type tgChat struct {
	ID int64 `json:"id"`
}

type tgUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type tgMessage struct {
	MessageID int    `json:"message_id"`
	Text      string `json:"text"`
	Caption   string `json:"caption"`
	Chat      tgChat `json:"chat"`
	From      tgUser `json:"from"`
}

type tgCallbackQuery struct {
	ID      string     `json:"id"`
	From    tgUser     `json:"from"`
	Data    string     `json:"data"`
	Message *tgMessage `json:"message,omitempty"`
}

type tgUpdate struct {
	UpdateID      int              `json:"update_id"`
	Message       *tgMessage       `json:"message,omitempty"`
	CallbackQuery *tgCallbackQuery `json:"callback_query,omitempty"`
}

type tgInlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

type tgInlineMarkup struct {
	InlineKeyboard [][]tgInlineButton `json:"inline_keyboard"`
}

type tgGetUpdatesResp struct {
	OK     bool       `json:"ok"`
	Result []tgUpdate `json:"result"`
}

type tgSendResp struct {
	OK     bool      `json:"ok"`
	Result tgMessage `json:"result"`
}

// ---------- Telegram API helpers ----------

func tgGetUpdates(botToken string, offset int) ([]tgUpdate, error) {
	apiURL := fmt.Sprintf(
		"https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30&allowed_updates=%s",
		botToken, offset, url.QueryEscape(`["message","callback_query"]`),
	)
	c := &http.Client{Timeout: 40 * time.Second}
	resp, err := c.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("getUpdates HTTP %d: %s", resp.StatusCode, string(body))
	}
	var r tgGetUpdatesResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	if !r.OK {
		return nil, fmt.Errorf("getUpdates !ok: %s", string(body))
	}
	return r.Result, nil
}

func tgAnswerCallback(botToken, callbackID, text string) error {
	api := fmt.Sprintf("https://api.telegram.org/bot%s/answerCallbackQuery", botToken)
	form := url.Values{}
	form.Set("callback_query_id", callbackID)
	if text != "" {
		form.Set("text", text)
	}
	resp, err := http.PostForm(api, form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}

func tgEditMessageCaption(botToken, chatID string, messageID int, caption string) error {
	api := fmt.Sprintf("https://api.telegram.org/bot%s/editMessageCaption", botToken)
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("message_id", strconv.Itoa(messageID))
	form.Set("caption", caption)
	form.Set("parse_mode", "HTML")
	// Pas de reply_markup → enlève les boutons (set "" or omit ?)
	form.Set("reply_markup", `{"inline_keyboard":[]}`)
	resp, err := http.PostForm(api, form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("editMessageCaption HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// sendTelegramPhotoWithButtons : envoie photo+caption+inline buttons et renvoie message_id.
func sendTelegramPhotoWithButtons(botToken, chatID, photoURL, caption string, buttons [][]tgInlineButton, silent bool) (int, error) {
	api := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", botToken)
	if photoURL == "" {
		return sendTelegramTextWithButtons(botToken, chatID, caption, buttons, silent)
	}
	markupJSON, _ := json.Marshal(tgInlineMarkup{InlineKeyboard: buttons})
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("photo", photoURL)
	form.Set("caption", caption)
	form.Set("parse_mode", "HTML")
	if silent {
		form.Set("disable_notification", "true")
	}
	if len(buttons) > 0 {
		form.Set("reply_markup", string(markupJSON))
	}
	resp, err := http.PostForm(api, form)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		// Fallback texte
		return sendTelegramTextWithButtons(botToken, chatID, caption, buttons, silent)
	}
	var r tgSendResp
	if err := json.Unmarshal(body, &r); err != nil {
		return 0, err
	}
	return r.Result.MessageID, nil
}

func sendTelegramTextWithButtons(botToken, chatID, text string, buttons [][]tgInlineButton, silent bool) (int, error) {
	api := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	markupJSON, _ := json.Marshal(tgInlineMarkup{InlineKeyboard: buttons})
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	form.Set("parse_mode", "HTML")
	form.Set("disable_web_page_preview", "true")
	if silent {
		form.Set("disable_notification", "true")
	}
	if len(buttons) > 0 {
		form.Set("reply_markup", string(markupJSON))
	}
	resp, err := http.PostForm(api, form)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	var r tgSendResp
	if err := json.Unmarshal(body, &r); err != nil {
		return 0, err
	}
	return r.Result.MessageID, nil
}

// ---------- Bot state (offset for getUpdates) ----------

func botStateGet(db *sql.DB, key string) (string, error) {
	var v string
	err := db.QueryRow("SELECT value FROM bot_state WHERE key=?", key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func botStateSet(db *sql.DB, key, value string) error {
	_, err := db.Exec(
		"INSERT INTO bot_state(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",
		key, value,
	)
	return err
}

// ---------- Bot loop ----------

func runTelegramBot(cfg *Config, db *sql.DB, tmdbClient *tmdb.Client, stop <-chan struct{}) {
	offsetStr, _ := botStateGet(db, "tg_offset")
	offset, _ := strconv.Atoi(offsetStr)
	log.Printf("[auto-post] Telegram bot started (offset=%d)", offset)

	for {
		select {
		case <-stop:
			return
		default:
		}

		updates, err := tgGetUpdates(cfg.Telegram.BotToken, offset)
		if err != nil {
			log.Printf("[auto-post] tg getUpdates err: %v", err)
			// Retry après pause pour pas spammer en cas de panne
			select {
			case <-time.After(10 * time.Second):
			case <-stop:
				return
			}
			continue
		}
		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			handleUpdate(cfg, db, tmdbClient, u)
		}
		_ = botStateSet(db, "tg_offset", strconv.Itoa(offset))
	}
}

func handleUpdate(cfg *Config, db *sql.DB, tmdbClient *tmdb.Client, u tgUpdate) {
	if u.CallbackQuery != nil {
		handleCallback(cfg, db, tmdbClient, u.CallbackQuery)
		return
	}
	if u.Message != nil {
		handleMessage(cfg, db, tmdbClient, u.Message)
	}
}

// handleCallback : button press. Format : "<action>:<job_id>[:<arg>]"
func handleCallback(cfg *Config, db *sql.DB, tmdbClient *tmdb.Client, cb *tgCallbackQuery) {
	parts := strings.Split(cb.Data, ":")
	if len(parts) < 2 {
		_ = tgAnswerCallback(cfg.Telegram.BotToken, cb.ID, "callback invalide")
		return
	}
	action := parts[0]
	jobID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		_ = tgAnswerCallback(cfg.Telegram.BotToken, cb.ID, "job_id invalide")
		return
	}

	switch action {
	case "confirm":
		// Confirme le best match déjà en DB
		_, _ = db.Exec("UPDATE jobs SET tmdb_status='confirmed' WHERE id=?", jobID)
		title, year := getJobTMDBInfo(db, jobID)
		_ = tgAnswerCallback(cfg.Telegram.BotToken, cb.ID, "✅ Confirmé")
		if cb.Message != nil {
			newCap := fmt.Sprintf("✅ <b>Confirmé</b> : %s (%s)", htmlEscape(title), htmlEscape(year))
			_ = tgEditMessageCaption(cfg.Telegram.BotToken, cfg.Telegram.ChatID, cb.Message.MessageID, newCap)
		}
		log.Printf("[auto-post] CB confirm job=%d → %s (%s)", jobID, title, year)

	case "alt":
		// "alt:<job_id>:<index>" → switch to that alternative
		if len(parts) < 3 {
			_ = tgAnswerCallback(cfg.Telegram.BotToken, cb.ID, "index manquant")
			return
		}
		altIdx, _ := strconv.Atoi(parts[2])
		altsJSON := getJobAlts(db, jobID)
		var alts []tmdb.Movie
		if err := json.Unmarshal([]byte(altsJSON), &alts); err != nil || altIdx < 0 || altIdx >= len(alts) {
			_ = tgAnswerCallback(cfg.Telegram.BotToken, cb.ID, "alt introuvable")
			return
		}
		chosen := alts[altIdx]
		_, _ = db.Exec(
			"UPDATE jobs SET tmdb_id=?, tmdb_title=?, tmdb_year=?, tmdb_poster=?, tmdb_status='confirmed' WHERE id=?",
			chosen.ID, chosen.DisplayTitle(), chosen.Year(), chosen.PosterURL(), jobID,
		)
		_ = tgAnswerCallback(cfg.Telegram.BotToken, cb.ID, "✅ Switch alt "+strconv.Itoa(altIdx+1))
		if cb.Message != nil {
			newCap := fmt.Sprintf("✅ <b>Confirmé (alt %d)</b> : %s (%s)", altIdx+1, htmlEscape(chosen.DisplayTitle()), htmlEscape(chosen.Year()))
			_ = tgEditMessageCaption(cfg.Telegram.BotToken, cfg.Telegram.ChatID, cb.Message.MessageID, newCap)
		}
		log.Printf("[auto-post] CB alt%d job=%d → %s (%s)", altIdx+1, jobID, chosen.DisplayTitle(), chosen.Year())

	case "skip":
		_, _ = db.Exec("UPDATE jobs SET tmdb_status='skipped' WHERE id=?", jobID)
		_ = tgAnswerCallback(cfg.Telegram.BotToken, cb.ID, "❌ Skip")
		if cb.Message != nil {
			newCap := "❌ <b>Skipped</b> par utilisateur"
			_ = tgEditMessageCaption(cfg.Telegram.BotToken, cfg.Telegram.ChatID, cb.Message.MessageID, newCap)
		}
		log.Printf("[auto-post] CB skip job=%d", jobID)

	case "manual":
		_, _ = db.Exec("UPDATE jobs SET tmdb_status='awaiting_manual_id' WHERE id=?", jobID)
		_ = botStateSet(db, "awaiting_manual_for_job", strconv.FormatInt(jobID, 10))
		_ = tgAnswerCallback(cfg.Telegram.BotToken, cb.ID, "✏️ Envoie l'ID TMDB en réponse")
		releaseTitle := getJobTitle(db, jobID)
		msg := fmt.Sprintf("✏️ Envoie l'ID TMDB pour :\n<code>%s</code>\n\n(juste le numéro, ex: 27205)", htmlEscape(releaseTitle))
		_, _ = sendTelegramTextWithButtons(cfg.Telegram.BotToken, cfg.Telegram.ChatID, msg, nil, false)
		log.Printf("[auto-post] CB manual job=%d (awaiting input)", jobID)

	default:
		_ = tgAnswerCallback(cfg.Telegram.BotToken, cb.ID, "action inconnue")
	}
}

// handleMessage : message texte (potentiellement réponse à demande d'ID manuel)
func handleMessage(cfg *Config, db *sql.DB, tmdbClient *tmdb.Client, msg *tgMessage) {
	// On ne traite que les messages du chat configuré (sécurité)
	if strconv.FormatInt(msg.Chat.ID, 10) != cfg.Telegram.ChatID {
		return
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	// Si on attend un ID manuel et que le texte est numérique → traite-le
	awaiting, _ := botStateGet(db, "awaiting_manual_for_job")
	if awaiting != "" {
		tmdbID, err := strconv.Atoi(text)
		if err == nil && tmdbID > 0 {
			jobID, _ := strconv.ParseInt(awaiting, 10, 64)
			applyManualTMDBID(cfg, db, tmdbClient, jobID, tmdbID)
			_ = botStateSet(db, "awaiting_manual_for_job", "")
			return
		}
	}
	// Sinon : ignoré pour l'instant (slash commands à ajouter plus tard)
}

func applyManualTMDBID(cfg *Config, db *sql.DB, tmdbClient *tmdb.Client, jobID int64, tmdbID int) {
	// Retry court : 2 essais avec 1s de pause (proxy parfois lent)
	var movie *tmdb.Movie
	var err error
	for attempt := 1; attempt <= 2; attempt++ {
		movie, err = tmdbClient.GetByID(tmdbID, "movie")
		if err == nil && movie != nil && movie.ID > 0 {
			break
		}
		log.Printf("[auto-post] manual ID lookup attempt %d/2 failed (id=%d): err=%v movie=%v", attempt, tmdbID, err, movie)
		time.Sleep(1 * time.Second)
	}
	if err != nil || movie == nil || movie.ID == 0 {
		_, _ = sendTelegramTextWithButtons(cfg.Telegram.BotToken, cfg.Telegram.ChatID,
			fmt.Sprintf("❌ TMDB ID %d introuvable: %v", tmdbID, err), nil, false)
		_, _ = db.Exec("UPDATE jobs SET tmdb_status='no_match' WHERE id=?", jobID)
		log.Printf("[auto-post] manual ID %d failed for job=%d: %v", tmdbID, jobID, err)
		return
	}
	_, _ = db.Exec(
		"UPDATE jobs SET tmdb_id=?, tmdb_title=?, tmdb_year=?, tmdb_poster=?, tmdb_status='confirmed' WHERE id=?",
		movie.ID, movie.DisplayTitle(), movie.Year(), movie.PosterURL(), jobID,
	)
	caption := fmt.Sprintf("✅ <b>Confirmé manuellement</b>\n\n<b>%s</b> (%s)\nTMDB: %d", htmlEscape(movie.DisplayTitle()), htmlEscape(movie.Year()), movie.ID)
	_, _ = sendTelegramPhotoWithButtons(cfg.Telegram.BotToken, cfg.Telegram.ChatID, movie.PosterURL(), caption, nil, false)
	log.Printf("[auto-post] manual ID %d → job=%d %s (%s)", tmdbID, jobID, movie.DisplayTitle(), movie.Year())
}

// ---------- Helpers DB ----------

func getJobTitle(db *sql.DB, jobID int64) string {
	var t string
	_ = db.QueryRow("SELECT title FROM jobs WHERE id=?", jobID).Scan(&t)
	return t
}

func getJobTMDBInfo(db *sql.DB, jobID int64) (title, year string) {
	_ = db.QueryRow("SELECT COALESCE(tmdb_title,''), COALESCE(tmdb_year,'') FROM jobs WHERE id=?", jobID).Scan(&title, &year)
	return
}

func getJobAlts(db *sql.DB, jobID int64) string {
	var s string
	_ = db.QueryRow("SELECT COALESCE(tmdb_alts_json,'[]') FROM jobs WHERE id=?", jobID).Scan(&s)
	return s
}
