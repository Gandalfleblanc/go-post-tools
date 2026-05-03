package main

// Listener IRC pour bot d'annonces (UNFR_BOT sur Recycled-IRC).
// Format des messages parsé :
//   [CATEGORIE] [ filename ] [ taille ] [ ID_unfr ]
// L'URL NZB est reconstruite via : <unfr_get_url>?id=<ID>&key=<unfr_key>
//
// Pas de lib IRC externe : connexion TCP+TLS directe, gestion PING/PONG +
// invite handshake + auto-rejoin des canaux invités.

import (
	"bufio"
	"crypto/tls"
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
)

// Annonce parsée du bot
type ircAnnounce struct {
	Category string
	Filename string
	Size     string
	UnfrID   string
}

func (a ircAnnounce) GUID() string {
	return "unfr-irc-" + a.UnfrID
}

func (a ircAnnounce) NZBURL(getURL, key string) string {
	if getURL == "" {
		getURL = "https://unfr.pw/get.php"
	}
	return fmt.Sprintf("%s?id=%s&key=%s", getURL, a.UnfrID, key)
}

// Regex pour parser : "[CAT] [ filename ] [ size ] [ ID ]"
// Tolère espaces variables autour des contenus.
var (
	announceRE = regexp.MustCompile(`^\[([^\]]+)\]\s*\[\s*(.+?)\s*\]\s*\[\s*(.+?)\s*\]\s*\[\s*(.+?)\s*\]\s*$`)
)

func parseAnnounce(text string) (ircAnnounce, bool) {
	m := announceRE.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return ircAnnounce{}, false
	}
	return ircAnnounce{
		Category: strings.TrimSpace(m[1]),
		Filename: strings.TrimSpace(m[2]),
		Size:     strings.TrimSpace(m[3]),
		UnfrID:   strings.TrimSpace(m[4]),
	}, true
}

// Strip prefix mode chars ('~', '&', '@', '%', '+') du nick (modes ops/voice).
func stripNickPrefix(s string) string {
	for len(s) > 0 && strings.ContainsRune("~&@%+", rune(s[0])) {
		s = s[1:]
	}
	return s
}

// runIRCListener : connexion + boucle de lecture. Bloquant. Reconnecte
// automatiquement avec backoff en cas de perte.
func runIRCListener(cfg *Config, db *sql.DB, stop <-chan struct{}) {
	backoff := 5 * time.Second
	for {
		select {
		case <-stop:
			return
		default:
		}

		err := ircSessionOnce(cfg, db, stop)
		if err != nil {
			log.Printf("[irc] session ended: %v (reconnect dans %s)", err, backoff)
		}

		select {
		case <-stop:
			return
		case <-time.After(backoff):
		}
		// Backoff plafonné à 5 min
		if backoff < 5*time.Minute {
			backoff *= 2
		}
	}
}

// ircSessionOnce : 1 connexion + handshake + boucle messages. Retourne quand
// la connexion est perdue ou que stop est fermé.
func ircSessionOnce(cfg *Config, db *sql.DB, stop <-chan struct{}) error {
	addr := fmt.Sprintf("%s:%d", cfg.IRC.Host, cfg.IRC.Port)
	log.Printf("[irc] connect TLS %s as %s", addr, cfg.IRC.Nick)

	dialer := &tls.Dialer{
		Config: &tls.Config{
			ServerName:         cfg.IRC.Host,
			InsecureSkipVerify: cfg.IRC.InsecureSkipVerify,
		},
	}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	// On configure des deadlines de lecture périodiques pour détecter freeze
	r := bufio.NewReader(conn)

	send := func(line string) error {
		_, err := conn.Write([]byte(line + "\r\n"))
		if err == nil {
			log.Printf("[irc] >> %s", line)
		}
		return err
	}

	// Handshake
	if err := send("NICK " + cfg.IRC.Nick); err != nil {
		return err
	}
	if err := send(fmt.Sprintf("USER %s 0 * :%s", cfg.IRC.Nick, cfg.IRC.Nick)); err != nil {
		return err
	}

	// Goroutine pour fermer si stop arrive
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-stop:
			conn.Close()
		case <-done:
		}
	}()

	registered := false
	inviteSent := false

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}

		// Log raw IRC seulement si verbose (silencieux par défaut)
		if cfg.IRC.LogRaw {
			log.Printf("[irc] << %s", line)
		}

		// Réponse PING immédiate
		if strings.HasPrefix(line, "PING ") {
			_ = send("PONG " + line[5:])
			continue
		}

		// Parse :prefix command params...
		var prefix, cmd string
		var params []string
		s := line
		if strings.HasPrefix(s, ":") {
			sp := strings.SplitN(s[1:], " ", 2)
			prefix = sp[0]
			if len(sp) > 1 {
				s = sp[1]
			} else {
				s = ""
			}
		}
		// command + params (params se termine par ":trailing" éventuel)
		var trailing string
		if i := strings.Index(s, " :"); i >= 0 {
			trailing = s[i+2:]
			s = s[:i]
		}
		parts := strings.Fields(s)
		if len(parts) > 0 {
			cmd = parts[0]
			params = parts[1:]
		}
		if trailing != "" {
			params = append(params, trailing)
		}

		switch cmd {
		case "001":
			// Welcome — connexion établie
			registered = true
			log.Printf("[irc] registered (welcome)")
			if !inviteSent {
				// Envoie la commande d'invite à UNFR_BOT
				_ = send(fmt.Sprintf("PRIVMSG %s :invite %s %s",
					cfg.IRC.InviteBot, cfg.IRC.Nick, cfg.IRC.UnfrKey))
				inviteSent = true
			}
		case "INVITE":
			// :BOT INVITE OurNick :#channel
			if len(params) >= 2 {
				ch := params[1]
				log.Printf("[irc] invited to %s, joining", ch)
				_ = send("JOIN " + ch)
			}
		case "JOIN":
			if len(params) >= 1 {
				log.Printf("[irc] joined %s", params[0])
			}
		case "PRIVMSG":
			// :nick!user@host PRIVMSG #chan :text
			if len(params) >= 2 {
				target := params[0]
				text := params[len(params)-1]
				nick := stripNickPrefix(strings.SplitN(prefix, "!", 2)[0])
				if strings.EqualFold(nick, cfg.IRC.AnnounceBot) && strings.HasPrefix(target, "#") {
					if a, ok := parseAnnounce(text); ok {
						handleIRCAnnounce(cfg, db, a)
					} else if cfg.IRC.LogRaw {
						log.Printf("[irc] msg from %s ne matche pas le format annonce: %q", nick, text)
					}
				}
			}
		case "NICK":
			// rare : changement de nick imposé par le serveur
		case "ERROR":
			return fmt.Errorf("server ERROR: %s", strings.Join(params, " "))
		}

		_ = registered // (peut servir plus tard pour timeouts handshake)
	}
}

// handleIRCAnnounce : traite une annonce reçue. Identique à processFeed pour
// les RSS items, mais sans GUID externe — on en construit un depuis l'ID unfr.
func handleIRCAnnounce(cfg *Config, db *sql.DB, a ircAnnounce) {
	if isPaused() {
		return
	}
	guid := a.GUID()

	// Anti-doublon
	var existing string
	err := db.QueryRow("SELECT guid FROM seen_items WHERE guid = ?", guid).Scan(&existing)
	if err == nil {
		return // déjà vu
	}
	if err != sql.ErrNoRows {
		log.Printf("[irc] DB query err: %v", err)
		return
	}

	team := extractTeam(a.Filename)
	now := time.Now().Unix()
	link := a.NZBURL(cfg.IRC.UnfrGetURL, cfg.IRC.UnfrKey)

	// Reuse RSSItem-like struct pour réutiliser passesFilters / formatNotif
	pseudoItem := RSSItem{
		Title:    a.Filename,
		Link:     link,
		Category: a.Category,
		GUID:     guid,
	}
	ok, reason := passesFilters(cfg, pseudoItem, team)

	_, err = db.Exec(
		"INSERT INTO seen_items (guid, title, category, team, link, seen_at) VALUES (?, ?, ?, ?, ?, ?)",
		guid, a.Filename, a.Category, team, link, now,
	)
	if err != nil {
		log.Printf("[irc] DB insert err: %v", err)
		return
	}

	if !ok {
		log.Printf("[irc] SKIP %s (%s)", a.Filename, reason)
		return
	}

	log.Printf("[irc] SUBMIT %s [team=%s cat=%s size=%s]", a.Filename, team, a.Category, a.Size)
	nzoID, err := sabAddURL(cfg, link, a.Filename)
	if err != nil {
		log.Printf("[irc] SAB submit err: %v", err)
		_, _ = db.Exec(
			"INSERT INTO jobs (guid, title, category, team, nzb_url, status, error, submitted_at) VALUES (?, ?, ?, ?, ?, 'submit_failed', ?, ?)",
			guid, a.Filename, a.Category, team, link, err.Error(), now,
		)
		notifyFailed(cfg, a.Filename, "SAB submit error: "+err.Error())
		return
	}

	_, err = db.Exec(
		"INSERT INTO jobs (guid, title, category, team, nzb_url, nzo_id, status, submitted_at) VALUES (?, ?, ?, ?, ?, ?, 'downloading', ?)",
		guid, a.Filename, a.Category, team, link, nzoID, now,
	)
	if err != nil {
		log.Printf("[irc] DB job insert err: %v", err)
	}
	notifySubmitted(cfg, pseudoItem, team)
}
