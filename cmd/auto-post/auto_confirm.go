package main

// Logique des auto_confirm rules : skip le clic Discord pour les teams
// de confiance avec un score TMDB élevé.

import "strings"

// applyAutoConfirm : true si on doit auto-confirmer ce job (bypasse le pending).
// Conditions :
//   - cfg.AutoConfirm.Enabled
//   - score TMDB >= MinScore
//   - team du release dans TrustedTeams (ou liste vide = toutes whitelistées)
//   - via_bulk_import == 0 OU ApplyCatchup == true
func applyAutoConfirm(cfg *Config, title string, score float64, viaBulk bool) bool {
	if !cfg.AutoConfirm.Enabled {
		return false
	}
	minScore := cfg.AutoConfirm.MinScore
	if minScore <= 0 {
		minScore = 0.90
	}
	if score < minScore {
		return false
	}
	if viaBulk && !cfg.AutoConfirm.ApplyCatchup {
		return false
	}
	team := strings.ToLower(extractTeam(title))
	if team == "" {
		return false
	}
	// Liste vide = toutes les teams whitelistées éligibles
	if len(cfg.AutoConfirm.TrustedTeams) == 0 {
		for _, t := range cfg.Filters.AllowedTeams {
			if strings.ToLower(t) == team {
				return true
			}
		}
		return false
	}
	// Sinon : team doit être dans TrustedTeams
	for _, t := range cfg.AutoConfirm.TrustedTeams {
		if strings.ToLower(t) == team {
			return true
		}
	}
	return false
}
