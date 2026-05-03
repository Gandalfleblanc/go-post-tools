package main

// Discord bot pour notifications + validation interactive.
// Remplace ntfy comme backend principal : meilleur UX (boutons natifs qui se
// grisent après tap, edit du message, modals pour saisie ID TMDB).
//
// Connexion via Gateway WebSocket — pas besoin d'endpoint HTTPS public.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"go-post-tools/internal/tmdb"
)

type discordBot struct {
	session    *discordgo.Session
	cfg        *Config
	db         *sql.DB
	tmdbClient *tmdb.Client
	mu         sync.Mutex
}

var globalDiscord *discordBot

// initDiscord : ouvre la session Discord (WebSocket), enregistre les handlers.
// Stocké dans globalDiscord pour usage par les notify*().
func initDiscord(cfg *Config, db *sql.DB, tmdbClient *tmdb.Client) error {
	if cfg.Discord.Token == "" || cfg.Discord.ChannelID == "" {
		return fmt.Errorf("discord non configuré (token ou channel_id manquant)")
	}
	s, err := discordgo.New("Bot " + cfg.Discord.Token)
	if err != nil {
		return err
	}
	bot := &discordBot{
		session:    s,
		cfg:        cfg,
		db:         db,
		tmdbClient: tmdbClient,
	}
	s.AddHandler(bot.onReady)
	s.AddHandler(bot.onInteraction)
	s.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages
	if err := s.Open(); err != nil {
		return err
	}
	globalDiscord = bot
	return nil
}

func closeDiscord() {
	if globalDiscord != nil && globalDiscord.session != nil {
		_ = globalDiscord.session.Close()
	}
}

func (b *discordBot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	log.Printf("[discord] connecté en tant que %s#%s", r.User.Username, r.User.Discriminator)
}

// ---------- Envoi messages ----------

// sendDiscordSimple : message texte simple (boot, recap, etc.)
func sendDiscordSimple(title, body string, color int) error {
	if globalDiscord == nil {
		return fmt.Errorf("discord non initialisé")
	}
	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: body,
		Color:       color,
	}
	_, err := globalDiscord.session.ChannelMessageSendEmbed(globalDiscord.cfg.Discord.ChannelID, embed)
	return err
}

// sendDiscordTMDB : notif de validation TMDB avec poster + boutons.
// Retourne le message_id Discord (pour edit ultérieur après action).
func sendDiscordTMDB(jobID int64, release string, res TMDBResult) (string, error) {
	if globalDiscord == nil {
		return "", fmt.Errorf("discord non initialisé")
	}
	id := strconv.FormatInt(jobID, 10)

	var color int
	var title string
	switch res.Status {
	case "high_confidence":
		color = 0x2e7d32
		title = "✅ Match TMDB confiant"
	case "pending":
		color = 0xed6c02
		title = "🤔 Confirmation requise"
	case "no_match":
		color = 0xb71c1c
		title = "❌ Aucun match TMDB"
	default:
		color = 0xef6c00
		title = "⚠️ Erreur TMDB"
	}

	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: "**Release** : `" + release + "`",
		Color:       color,
		Fields:      []*discordgo.MessageEmbedField{},
	}

	if res.Best != nil {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Best match",
			Value:  fmt.Sprintf("**%s** (%s) — score `%.2f`\n[Fiche TMDB](https://www.themoviedb.org/movie/%d)", res.Best.DisplayTitle(), res.Best.Year(), res.Score, res.Best.ID),
			Inline: false,
		})
		if res.Best.PosterURL() != "" {
			embed.Image = &discordgo.MessageEmbedImage{URL: res.Best.PosterURL()}
		}
		if res.Best.Overview != "" {
			ov := res.Best.Overview
			if len(ov) > 250 {
				ov = ov[:250] + "…"
			}
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:   "Synopsis",
				Value:  ov,
				Inline: false,
			})
		}
	}
	if len(res.Alts) > 0 {
		alts := ""
		for i, a := range res.Alts {
			if i >= 3 {
				break
			}
			alts += fmt.Sprintf("• %s (%s) — [TMDB](https://www.themoviedb.org/movie/%d)\n", a.DisplayTitle(), a.Year(), a.ID)
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Alternatives",
			Value:  alts,
			Inline: false,
		})
	}

	// Boutons : adaptés selon le statut
	var buttons []discordgo.MessageComponent
	switch res.Status {
	case "high_confidence":
		// Auto-confirmé : juste un bouton "Annuler" si l'user veut bloquer
		// Sinon pas d'interaction nécessaire
		// On laisse skip + manual au cas où
		buttons = []discordgo.MessageComponent{
			discordgo.Button{Label: "❌ Skip quand même", Style: discordgo.DangerButton, CustomID: "skip:" + id},
		}
	case "pending":
		row := []discordgo.MessageComponent{
			discordgo.Button{Label: "✅ Confirmer", Style: discordgo.SuccessButton, CustomID: "confirm:" + id},
		}
		for i := range res.Alts {
			if i >= 2 {
				break // max 5 buttons par row, on garde Confirm + Alt1 + Alt2 + Skip + Manual
			}
			row = append(row, discordgo.Button{
				Label:    fmt.Sprintf("🔄 Alt %d", i+1),
				Style:    discordgo.PrimaryButton,
				CustomID: fmt.Sprintf("alt:%s:%d", id, i),
			})
		}
		row = append(row, discordgo.Button{Label: "❌ Skip", Style: discordgo.DangerButton, CustomID: "skip:" + id})
		row = append(row, discordgo.Button{Label: "✏️ ID manuel", Style: discordgo.SecondaryButton, CustomID: "manual:" + id})
		buttons = []discordgo.MessageComponent{discordgo.ActionsRow{Components: row}}
	default: // no_match, error
		buttons = []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "✏️ ID manuel", Style: discordgo.SecondaryButton, CustomID: "manual:" + id},
			discordgo.Button{Label: "❌ Skip", Style: discordgo.DangerButton, CustomID: "skip:" + id},
		}}}
	}
	if len(buttons) > 0 {
		if _, ok := buttons[0].(discordgo.ActionsRow); !ok {
			buttons = []discordgo.MessageComponent{discordgo.ActionsRow{Components: buttons}}
		}
	}

	msg, err := globalDiscord.session.ChannelMessageSendComplex(globalDiscord.cfg.Discord.ChannelID, &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: buttons,
	})
	if err != nil {
		return "", err
	}
	return msg.ID, nil
}

// editDiscordMessage : remplace le contenu d'un message après action user.
func editDiscordMessage(messageID, newTitle, newBody string, color int) error {
	if globalDiscord == nil || messageID == "" {
		return nil
	}
	embed := &discordgo.MessageEmbed{
		Title:       newTitle,
		Description: newBody,
		Color:       color,
	}
	empty := []discordgo.MessageComponent{}
	_, err := globalDiscord.session.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel:    globalDiscord.cfg.Discord.ChannelID,
		ID:         messageID,
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &empty,
	})
	return err
}

// ---------- Handler interactions (boutons + modals) ----------

func (b *discordBot) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionMessageComponent:
		b.handleButton(s, i)
	case discordgo.InteractionModalSubmit:
		b.handleModal(s, i)
	}
}

func (b *discordBot) handleButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	parts := strings.SplitN(data.CustomID, ":", 3)
	if len(parts) < 2 {
		return
	}
	action := parts[0]
	jobID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return
	}

	switch action {
	case "confirm":
		_, _ = b.db.Exec("UPDATE jobs SET tmdb_status='confirmed' WHERE id=?", jobID)
		siblings := countLinkedSiblings(b.db, jobID)
		if siblings > 0 {
			_, _ = b.db.Exec("UPDATE jobs SET tmdb_status='confirmed' WHERE linked_to_job_id=? AND tmdb_status='linked_pending'", jobID)
		}
		title, year := getJobTMDBInfo(b.db, jobID)
		body := fmt.Sprintf("**%s** (%s)", title, year)
		if siblings > 0 {
			body += fmt.Sprintf("\n+%d versions liées", siblings)
		}
		_ = updateInteractionMessage(s, i, "✅ Confirmé", body, 0x2e7d32)
		log.Printf("[discord] confirm job=%d → %s (%s) +%d", jobID, title, year, siblings)

	case "skip":
		_, _ = b.db.Exec("UPDATE jobs SET tmdb_status='skipped' WHERE id=?", jobID)
		siblings := countLinkedSiblings(b.db, jobID)
		if siblings > 0 {
			_, _ = b.db.Exec("UPDATE jobs SET tmdb_status='skipped' WHERE linked_to_job_id=? AND tmdb_status='linked_pending'", jobID)
		}
		title := getJobTitle(b.db, jobID)
		body := title
		if siblings > 0 {
			body += fmt.Sprintf("\n+%d versions liées", siblings)
		}
		_ = updateInteractionMessage(s, i, "❌ Skipped", body, 0x424242)
		log.Printf("[discord] skip job=%d (%s) +%d", jobID, title, siblings)

	case "alt":
		if len(parts) < 3 {
			return
		}
		altIdx, _ := strconv.Atoi(parts[2])
		altsJSON := getJobAlts(b.db, jobID)
		var alts []tmdb.Movie
		if err := json.Unmarshal([]byte(altsJSON), &alts); err != nil || altIdx < 0 || altIdx >= len(alts) {
			return
		}
		chosen := alts[altIdx]
		_, _ = b.db.Exec(
			"UPDATE jobs SET tmdb_id=?, tmdb_title=?, tmdb_year=?, tmdb_poster=?, tmdb_status='confirmed' WHERE id=?",
			chosen.ID, chosen.DisplayTitle(), chosen.Year(), chosen.PosterURL(), jobID,
		)
		siblings := countLinkedSiblings(b.db, jobID)
		if siblings > 0 {
			_, _ = b.db.Exec(`UPDATE jobs SET
				tmdb_id=?, tmdb_title=?, tmdb_year=?, tmdb_poster=?, tmdb_status='confirmed'
				WHERE linked_to_job_id=? AND tmdb_status='linked_pending'`,
				chosen.ID, chosen.DisplayTitle(), chosen.Year(), chosen.PosterURL(), jobID)
		}
		body := fmt.Sprintf("**%s** (%s)", chosen.DisplayTitle(), chosen.Year())
		if siblings > 0 {
			body += fmt.Sprintf("\n+%d versions liées", siblings)
		}
		_ = updateInteractionMessage(s, i, fmt.Sprintf("✅ Alt %d confirmé", altIdx+1), body, 0x2e7d32)
		log.Printf("[discord] alt%d job=%d → %s (%s) +%d", altIdx+1, jobID, chosen.DisplayTitle(), chosen.Year(), siblings)

	case "manual":
		// Ouvre un modal pour saisie de l'ID TMDB
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID: fmt.Sprintf("manual_modal:%d", jobID),
				Title:    "ID TMDB manuel",
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "tmdb_id",
							Label:       "ID TMDB du film",
							Style:       discordgo.TextInputShort,
							Placeholder: "ex: 27205",
							Required:    true,
							MinLength:   1,
							MaxLength:   10,
						},
					}},
				},
			},
		})
		if err != nil {
			log.Printf("[discord] modal err: %v", err)
		}
	}
}

func (b *discordBot) handleModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	parts := strings.SplitN(data.CustomID, ":", 2)
	if len(parts) < 2 || parts[0] != "manual_modal" {
		return
	}
	jobID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return
	}
	// Récupère la valeur du champ
	var tmdbIDStr string
	for _, row := range data.Components {
		ar, ok := row.(*discordgo.ActionsRow)
		if !ok {
			continue
		}
		for _, comp := range ar.Components {
			ti, ok := comp.(*discordgo.TextInput)
			if !ok {
				continue
			}
			if ti.CustomID == "tmdb_id" {
				tmdbIDStr = ti.Value
			}
		}
	}
	tmdbID, err := strconv.Atoi(strings.TrimSpace(tmdbIDStr))
	if err != nil || tmdbID <= 0 {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ ID invalide",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Retry 3x sur erreur transient (proxy parfois lent / 522).
	// Si UKLM rate complètement, on tente un fallback serveurperso pour
	// vérifier que l'ID existe (scrape HTML, retourne juste l'ID).
	var movie *tmdb.Movie
	for attempt := 1; attempt <= 3; attempt++ {
		movie, err = b.tmdbClient.GetByID(tmdbID, "movie")
		if err == nil && movie != nil && movie.ID > 0 {
			break
		}
		log.Printf("[discord] manual ID attempt %d/3 (id=%d): err=%v movie=%v", attempt, tmdbID, err, movie)
		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	if err != nil || movie == nil || movie.ID == 0 {
		// Fallback : vérifier que l'ID existe via serveurperso (scrape HTML).
		// On ne récupère pas la fiche complète, juste une confirmation que l'ID
		// est connu — l'utilisateur ouvrira le lien pour voir.
		exists, spErr := serveurpersoCheckTMDB(tmdbID)
		hint := fmt.Sprintf("Ouvre https://www.serveurperso.com/stats/search.php?query=%d pour vérifier manuellement.", tmdbID)
		if spErr == nil && exists {
			hint = fmt.Sprintf("L'ID existe sur serveurperso mais UKLM ne répond pas — réessaie dans quelques minutes.\nhttps://www.themoviedb.org/movie/%d", tmdbID)
		}
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ TMDB ID %d introuvable après 3 essais : %v\n%s", tmdbID, err, hint),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	_, _ = b.db.Exec(
		"UPDATE jobs SET tmdb_id=?, tmdb_title=?, tmdb_year=?, tmdb_poster=?, tmdb_status='confirmed' WHERE id=?",
		movie.ID, movie.DisplayTitle(), movie.Year(), movie.PosterURL(), jobID,
	)
	siblings := countLinkedSiblings(b.db, jobID)
	if siblings > 0 {
		_, _ = b.db.Exec(`UPDATE jobs SET
			tmdb_id=?, tmdb_title=?, tmdb_year=?, tmdb_poster=?, tmdb_status='confirmed'
			WHERE linked_to_job_id=? AND tmdb_status='linked_pending'`,
			movie.ID, movie.DisplayTitle(), movie.Year(), movie.PosterURL(), jobID)
	}
	body := fmt.Sprintf("**%s** (%s) — TMDB %d", movie.DisplayTitle(), movie.Year(), movie.ID)
	if siblings > 0 {
		body += fmt.Sprintf("\n+%d versions liées", siblings)
	}
	// Edit le message d'origine (celui avec le bouton "ID manuel") pour le marquer
	// validé et retirer les boutons. Le user voit ainsi disparaître la demande
	// d'ID manuel et apparaître la fiche validée à sa place.
	editedEmbed := &discordgo.MessageEmbed{
		Title:       "✅ Confirmé manuellement",
		Description: body,
		Color:       0x2e7d32,
		Image:       &discordgo.MessageEmbedImage{URL: movie.PosterURL()},
	}
	emptyComponents := []discordgo.MessageComponent{}
	originalEdited := false
	if i.Message != nil && i.Message.ID != "" {
		_, editErr := s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel:    i.Message.ChannelID,
			ID:         i.Message.ID,
			Embeds:     &[]*discordgo.MessageEmbed{editedEmbed},
			Components: &emptyComponents,
		})
		if editErr != nil {
			log.Printf("[discord] manual edit original msg failed: %v", editErr)
		} else {
			originalEdited = true
		}
	}
	// Ack modal : si on a réussi à éditer l'original, on répond éphémère (silencieux).
	// Sinon on poste la fiche validée publiquement comme nouveau message.
	if originalEdited {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("✅ TMDB %d (%s) appliqué.", movie.ID, movie.DisplayTitle()),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	} else {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{editedEmbed},
			},
		})
	}
	log.Printf("[discord] manual job=%d → %s (%s) tmdb=%d", jobID, movie.DisplayTitle(), movie.Year(), movie.ID)
}

// updateInteractionMessage : edit le message d'origine avec un nouvel embed.
func updateInteractionMessage(s *discordgo.Session, i *discordgo.InteractionCreate, title, body string, color int) error {
	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: body,
		Color:       color,
	}
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: []discordgo.MessageComponent{}, // retire les boutons
		},
	})
}
