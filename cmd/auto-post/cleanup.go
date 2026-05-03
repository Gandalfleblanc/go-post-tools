package main

// Cleanup automatique des MKVs après 24h pour libérer le disque (77 GB sur le VPS).
// Critères : un job a un mkv_path, n'est plus en "en cours", et > 24h depuis son
// dernier changement d'état significatif.
//
// Cas couverts :
//   - tmdb_status='skipped'/'no_match'/'error' (pas de post Hydracker prévu)
//   - tmdb_status='confirmed' + hydracker_status final (posted/failed/partial/no_title/unknown_quality/lookup_failed)
// Cas exclus (on n'efface pas) :
//   - tmdb_status='pending' / 'awaiting_manual_id' (l'user n'a pas encore décidé)
//   - hydracker_status='posting' (post en cours)

import (
	"database/sql"
	"log"
	"os"
	"time"
)

const cleanupAgeSeconds = 24 * 3600

func pollCleanup(cfg *Config, db *sql.DB) error {
	cutoff := time.Now().Unix() - cleanupAgeSeconds

	rows, err := db.Query(`SELECT id, title, mkv_path FROM jobs
		WHERE mkv_path IS NOT NULL AND mkv_path != ''
		  AND COALESCE(cleanup_done_at, 0) = 0
		  AND (
		    (tmdb_status IN ('skipped','no_match','error')
		     AND COALESCE(tmdb_checked_at, 0) < ?)
		    OR
		    (tmdb_status = 'confirmed'
		     AND hydracker_status IS NOT NULL
		     AND hydracker_status NOT IN ('posting','')
		     AND COALESCE(hydracker_processed_at, 0) < ?)
		  )`, cutoff, cutoff)
	if err != nil {
		return err
	}
	defer rows.Close()

	type cleanJob struct {
		ID      int64
		Title   string
		MkvPath string
	}
	var jobs []cleanJob
	for rows.Next() {
		var j cleanJob
		if err := rows.Scan(&j.ID, &j.Title, &j.MkvPath); err != nil {
			return err
		}
		jobs = append(jobs, j)
	}
	rows.Close()

	now := time.Now().Unix()
	for _, j := range jobs {
		// Supprime le fichier (et le dossier parent si SAB l'a créé)
		if err := removeJobFiles(j.MkvPath); err != nil {
			log.Printf("[cleanup] FAIL job=%d %s : %v", j.ID, j.Title, err)
			// On marque quand même cleanup_done_at pour ne pas re-tenter en boucle
		} else {
			log.Printf("[cleanup] OK job=%d %s (24h+ post-state)", j.ID, j.Title)
		}
		_, _ = db.Exec("UPDATE jobs SET cleanup_done_at=? WHERE id=?", now, j.ID)
	}
	return nil
}

// removeJobFiles : supprime le MKV et son dossier parent SAB si vide.
// Format SAB : /var/lib/sabnzbd/complete/<release-name>/<release-name>.mkv
// → on supprime le dossier <release-name>/ entièrement.
func removeJobFiles(mkvPath string) error {
	info, err := os.Stat(mkvPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // déjà supprimé
		}
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	// Supprime le MKV
	if err := os.Remove(mkvPath); err != nil {
		return err
	}
	// Si le dossier parent est sous /var/lib/sabnzbd/complete/, le retirer
	// (RemoveAll au cas où il reste des .par2 ou logs de SAB)
	parent := parentDir(mkvPath)
	if parent != "" && parent != "/var/lib/sabnzbd/complete" {
		_ = os.RemoveAll(parent)
	}
	return nil
}

func parentDir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return ""
}
