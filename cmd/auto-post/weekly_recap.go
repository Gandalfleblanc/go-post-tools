package main

// Récap hebdo : envoyé tous les dimanches à 21h sur Discord.
// Compte les jobs traités sur les 7 derniers jours, par status / team / qualité.

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

func sendWeeklyRecap(cfg *Config, db *sql.DB) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour).Unix()

	type kv struct {
		k string
		v int
	}
	queryKV := func(sql string, args ...any) []kv {
		var out []kv
		rows, err := db.Query(sql, args...)
		if err != nil {
			return out
		}
		defer rows.Close()
		for rows.Next() {
			var s kv
			if rows.Scan(&s.k, &s.v) == nil {
				out = append(out, s)
			}
		}
		return out
	}
	count := func(sql string, args ...any) int {
		var n int
		_ = db.QueryRow(sql, args...).Scan(&n)
		return n
	}

	posted := count(`SELECT COUNT(*) FROM jobs WHERE hydracker_status='posted' AND hydracker_processed_at >= ?`, cutoff)
	dups := count(`SELECT COUNT(*) FROM jobs WHERE hydracker_status='dup_skipped' AND hydracker_processed_at >= ?`, cutoff)
	skipped := count(`SELECT COUNT(*) FROM jobs WHERE tmdb_status='skipped' AND COALESCE(hydracker_processed_at, tmdb_checked_at, 0) >= ?`, cutoff)
	failed := count(`SELECT COUNT(*) FROM jobs WHERE hydracker_status IN ('failed','partial','quality_skipped','no_title','unknown_quality','lookup_failed') AND hydracker_processed_at >= ?`, cutoff)

	teams := queryKV(`SELECT COALESCE(NULLIF(team,''),'(inconnu)'), COUNT(*) FROM jobs
		WHERE hydracker_status='posted' AND hydracker_processed_at >= ?
		GROUP BY 1 ORDER BY 2 DESC LIMIT 5`, cutoff)

	postedTitles := queryKV(`SELECT title, hydracker_nzb_id FROM jobs
		WHERE hydracker_status='posted' AND hydracker_processed_at >= ?
		ORDER BY hydracker_processed_at DESC LIMIT 10`, cutoff)

	var b strings.Builder
	fmt.Fprintf(&b, "📊 **Récap hebdo** (7 derniers jours)\n\n")
	fmt.Fprintf(&b, "✅ **%d posté(s)**\n", posted)
	fmt.Fprintf(&b, "⏭ %d doublon(s) skip\n", dups)
	fmt.Fprintf(&b, "❌ %d skip user\n", skipped)
	if failed > 0 {
		fmt.Fprintf(&b, "⚠️ %d échec(s)/skip qualité\n", failed)
	}

	if len(teams) > 0 {
		fmt.Fprintf(&b, "\n**Top teams**\n")
		for _, t := range teams {
			fmt.Fprintf(&b, "• %s : %d\n", t.k, t.v)
		}
	}

	if len(postedTitles) > 0 {
		fmt.Fprintf(&b, "\n**Films postés**\n")
		for _, t := range postedTitles {
			line := t.k
			if len(line) > 70 {
				line = line[:67] + "…"
			}
			fmt.Fprintf(&b, "• %s\n", line)
		}
	}

	emoji := "📊"
	if posted == 0 && dups == 0 {
		emoji = "💤"
	}
	if err := sendDiscordSimple(emoji+" Récap de la semaine", b.String(), 0x1565c0); err != nil {
		log.Printf("[recap] discord err: %v", err)
	} else {
		log.Printf("[recap] envoyé : %d posted, %d dup, %d skipped, %d failed", posted, dups, skipped, failed)
	}
}
