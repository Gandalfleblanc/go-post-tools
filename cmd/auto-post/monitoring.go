package main

// Self-monitoring : check périodique de la santé du système et alerte Discord
// si anomalie. Cooldown 6h par anomalie pour éviter le spam.

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	monitoringLastAlert   = map[string]time.Time{}
	monitoringLastAlertMu sync.Mutex
)

func runMonitoring(cfg *Config, db *sql.DB) {
	if !cfg.Monitoring.Enabled {
		return
	}

	check := func(key, name, detail string) {
		monitoringLastAlertMu.Lock()
		last, seen := monitoringLastAlert[key]
		fresh := !seen || time.Since(last) > 6*time.Hour
		if fresh {
			monitoringLastAlert[key] = time.Now()
		}
		monitoringLastAlertMu.Unlock()
		if !fresh {
			return // cooldown actif
		}
		log.Printf("[monitoring] alert: %s — %s", name, detail)
		_ = sendDiscordSimple("⚠️ Alerte : "+name, detail, 0xed6c02)
	}

	// 1. Aucun post depuis N jours ?
	days := cfg.Monitoring.AlertNoPostDays
	if days <= 0 {
		days = 3
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	var lastPosted int64
	_ = db.QueryRow(`SELECT COALESCE(MAX(hydracker_processed_at), 0) FROM jobs WHERE hydracker_status='posted'`).Scan(&lastPosted)
	if lastPosted > 0 && lastPosted < cutoff {
		ago := time.Since(time.Unix(lastPosted, 0)).Round(time.Hour)
		check("no_post", "Aucun post depuis "+ago.String(),
			"Vérifie l'IRC, les filtres, ou s'il y a juste pas de releases tes teams.")
	}

	// 2. Disque > N % utilisé ?
	pct := cfg.Monitoring.AlertDiskPercent
	if pct <= 0 {
		pct = 85
	}
	out, err := exec.Command("df", "-BG", "/").Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 1 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 5 {
				usePct, _ := strconv.Atoi(strings.TrimSuffix(fields[4], "%"))
				if usePct >= pct {
					check("disk_full", fmt.Sprintf("Disque %d%% utilisé", usePct),
						fmt.Sprintf("Total %s, utilisé %s, libre %s. Vérifie /var/lib/sabnzbd/complete et /var/backups.",
							fields[1], fields[2], fields[3]))
				}
			}
		}
	}

	// 3. Hydracker KO depuis N min ?
	minDown := cfg.Monitoring.AlertHydrackerDownMin
	if minDown <= 0 {
		minDown = 60
	}
	hydDownKey := "hydracker_down"
	{
		req, _ := http.NewRequest("GET", cfg.Hydracker.BaseURL+"/meta/quals", nil)
		req.Header.Set("Authorization", "Bearer "+cfg.Hydracker.Token)
		c := &http.Client{Timeout: 10 * time.Second}
		resp, err := c.Do(req)
		hydOK := err == nil && resp != nil && resp.StatusCode == 200
		if resp != nil {
			resp.Body.Close()
		}
		monitoringLastAlertMu.Lock()
		_, downSince := monitoringLastAlert["hydracker_first_fail"]
		if !hydOK {
			if !downSince {
				monitoringLastAlert["hydracker_first_fail"] = time.Now()
			}
			firstFail := monitoringLastAlert["hydracker_first_fail"]
			if time.Since(firstFail) > time.Duration(minDown)*time.Minute {
				monitoringLastAlertMu.Unlock()
				check(hydDownKey, "Hydracker API down",
					fmt.Sprintf("Pas de réponse 200 depuis %s.", time.Since(firstFail).Round(time.Minute)))
				return
			}
		} else {
			delete(monitoringLastAlert, "hydracker_first_fail")
		}
		monitoringLastAlertMu.Unlock()
	}

	// 4. Discord déconnecté ?
	if globalDiscord == nil || globalDiscord.session == nil || globalDiscord.session.State == nil {
		check("discord_down", "Discord bot déconnecté", "Vérifie le token / connexion réseau.")
	}

	// 5. Auto-post.service en running ?
	out, err = exec.Command("systemctl", "is-active", "auto-post.service").Output()
	if err == nil && strings.TrimSpace(string(out)) != "active" {
		// Bizarre, on est en train de tourner... mais bon, log.
	}
}
