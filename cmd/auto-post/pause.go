package main

// Pause logicielle : flag stocké dans bot_state (clé "paused"). Quand actif,
// tous les polls / handlers retournent immédiatement sans toucher à l'état.
// Le binaire reste vivant pour servir la web UI et permettre la reprise.
//
// Monitoring continue de tourner (on veut être alerté même en pause).
//
// Toggle via boutons sur la home (POST /admin/pause, POST /admin/resume).

import (
	"database/sql"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

var pausedAtomic atomic.Bool

// initPauseState : lit le flag persistant au boot et hydrate le cache atomic.
func initPauseState(db *sql.DB) {
	pausedAtomic.Store(readPausedDB(db))
}

func isPaused() bool {
	return pausedAtomic.Load()
}

func setPaused(db *sql.DB, paused bool) error {
	val := "0"
	if paused {
		val = "1"
	}
	_, err := db.Exec(
		"INSERT INTO bot_state(key,value) VALUES('paused',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",
		val,
	)
	if err != nil {
		return err
	}
	pausedAtomic.Store(paused)
	if paused {
		log.Printf("[pause] auto-post mis en pause via web UI")
	} else {
		log.Printf("[pause] auto-post repris via web UI")
	}
	return nil
}

func readPausedDB(db *sql.DB) bool {
	var v string
	if err := db.QueryRow("SELECT value FROM bot_state WHERE key='paused'").Scan(&v); err != nil {
		return false
	}
	return v == "1"
}

// pausedSleep : helper pour les polls — si paused retourne true et sleep court,
// l'appelant continue son loop sans rien faire.
func pausedSleep() bool {
	if isPaused() {
		time.Sleep(5 * time.Second)
		return true
	}
	return false
}

// handleAdminPauseToggle : POST /admin/pause ou /admin/resume → set le flag
// puis redirige vers la home. GET aussi accepté pour pouvoir cliquer un lien.
func handleAdminPauseToggle(db *sql.DB, w http.ResponseWriter, r *http.Request, paused bool) {
	if err := setPaused(db, paused); err != nil {
		http.Error(w, "setPaused: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if paused {
		_ = sendDiscordSimple("⏸ Auto Post en pause", "Pause déclenchée via web UI", 0xed6c02)
	} else {
		_ = sendDiscordSimple("▶ Auto Post repris", "Reprise déclenchée via web UI", 0x2e7d32)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
