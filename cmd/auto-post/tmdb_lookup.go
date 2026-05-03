package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go-post-tools/internal/parser"
	"go-post-tools/internal/tmdb"
)

// TMDBResult : sortie d'une recherche TMDB pour un release.
type TMDBResult struct {
	Status     string       // "high_confidence" | "pending" | "no_match" | "error"
	Best       *tmdb.Movie  // meilleur match (peut être nil)
	Alts       []tmdb.Movie // jusqu'à 3 alternatives
	Score      float64      // score du Best (0..1)
	Reason     string       // explication courte
	ParsedInfo *parser.FileInfo
}

const (
	thresholdHigh    = 0.90 // au-dessus → auto-post sans demander
	thresholdPending = 0.70 // entre les deux → notif confirmation
)

// lookupTMDB : parse le filename, query le proxy TMDB, score les candidats,
// retourne un verdict.
func lookupTMDB(client *tmdb.Client, filename string) TMDBResult {
	info := parser.ParseFilename(filename)
	res := TMDBResult{ParsedInfo: info}

	if info.Title == "" {
		res.Status = "error"
		res.Reason = "title vide après parsing"
		return res
	}

	// Construit la query : "Title YYYY". Si pas d'année, on tente sans —
	// le proxy peut refuser.
	query := info.Title
	if info.Year != "" {
		query = info.Title + " " + info.Year
	}

	candidates, err := client.Search(query)
	if err != nil {
		// Le proxy renvoie une erreur "Not found in localized and original titles
		// database" → on considère ça comme un no_match propre.
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			res.Status = "no_match"
			res.Reason = "TMDB ne connaît pas ce film: " + query
			return res
		}
		if info.Year == "" {
			res.Status = "error"
			res.Reason = "search failed (pas d'année dans le filename): " + err.Error()
			return res
		}
		res.Status = "error"
		res.Reason = "search failed: " + err.Error()
		return res
	}

	if len(candidates) == 0 {
		res.Status = "no_match"
		res.Reason = "aucun candidat TMDB pour " + query
		return res
	}

	// Garde uniquement les films (le user veut films seulement)
	movies := make([]tmdb.Movie, 0, len(candidates))
	for _, c := range candidates {
		if c.MediaType == "" || c.MediaType == "movie" {
			movies = append(movies, c)
		}
	}
	if len(movies) == 0 {
		res.Status = "no_match"
		res.Reason = "candidats trouvés mais aucun film"
		return res
	}

	// Score chaque candidat
	type scored struct {
		m     tmdb.Movie
		score float64
	}
	scoredList := make([]scored, len(movies))
	for i, m := range movies {
		scoredList[i] = scored{m: m, score: scoreCandidate(info, m)}
	}
	sort.SliceStable(scoredList, func(i, j int) bool {
		return scoredList[i].score > scoredList[j].score
	})

	best := scoredList[0]
	res.Best = &best.m
	res.Score = best.score
	for i := 1; i < len(scoredList) && i < 4; i++ {
		res.Alts = append(res.Alts, scoredList[i].m)
	}

	switch {
	case best.score >= thresholdHigh:
		res.Status = "high_confidence"
		res.Reason = fmt.Sprintf("score %.2f", best.score)
	case best.score >= thresholdPending:
		res.Status = "pending"
		res.Reason = fmt.Sprintf("score %.2f (en-dessous du seuil %.2f)", best.score, thresholdHigh)
	default:
		res.Status = "no_match"
		res.Reason = fmt.Sprintf("meilleur score trop bas: %.2f", best.score)
	}
	return res
}

// scoreCandidate : 0..1. Combine similarité title et match année.
func scoreCandidate(info *parser.FileInfo, m tmdb.Movie) float64 {
	titleSim := titleSimilarity(info.Title, m.DisplayTitle())
	yearMatch := yearScore(info.Year, m.Year())
	// Pondération : titre 70%, année 30%
	return 0.7*titleSim + 0.3*yearMatch
}

// titleSimilarity : normalise puis applique Dice coefficient sur les bigrams.
// Bonus : si une chaîne est strictement contenue dans l'autre, score min 0.85.
func titleSimilarity(a, b string) float64 {
	na, nb := normalizeTitle(a), normalizeTitle(b)
	if na == "" || nb == "" {
		return 0
	}
	if na == nb {
		return 1.0
	}
	// Substring containment → fort signal
	if strings.Contains(na, nb) || strings.Contains(nb, na) {
		base := dice(na, nb)
		if base < 0.85 {
			return 0.85
		}
		return base
	}
	return dice(na, nb)
}

// yearScore : 1.0 si match exact, 0.6 si ±1, 0 sinon. 0.5 si l'un des deux manque.
func yearScore(a, b string) float64 {
	if a == "" || b == "" {
		return 0.5
	}
	ya, _ := strconv.Atoi(a)
	yb, _ := strconv.Atoi(b)
	if ya == 0 || yb == 0 {
		return 0.5
	}
	diff := ya - yb
	if diff < 0 {
		diff = -diff
	}
	switch {
	case diff == 0:
		return 1.0
	case diff == 1:
		return 0.6
	default:
		return 0
	}
}

var nonAlpha = regexp.MustCompile(`[^a-z0-9]+`)

// normalizeTitle : lowercase, supprime accents basiques, ne garde que [a-z0-9].
func normalizeTitle(s string) string {
	s = strings.ToLower(s)
	// remplace les accents fréquents (le proxy renvoie souvent du Latin-1)
	rep := strings.NewReplacer(
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"à", "a", "â", "a", "ä", "a",
		"î", "i", "ï", "i",
		"ô", "o", "ö", "o",
		"ù", "u", "û", "u", "ü", "u",
		"ç", "c", "ñ", "n",
	)
	s = rep.Replace(s)
	s = nonAlpha.ReplaceAllString(s, "")
	return s
}

// dice : Dice-Sørensen coefficient sur character bigrams.
// Plus tolérant que Levenshtein pour les variations de typo / mots manquants.
func dice(a, b string) float64 {
	if len(a) < 2 || len(b) < 2 {
		if a == b {
			return 1
		}
		return 0
	}
	bigrams := func(s string) map[string]int {
		m := make(map[string]int)
		for i := 0; i < len(s)-1; i++ {
			m[s[i:i+2]]++
		}
		return m
	}
	ba, bb := bigrams(a), bigrams(b)
	inter := 0
	for k, va := range ba {
		if vb, ok := bb[k]; ok {
			if va < vb {
				inter += va
			} else {
				inter += vb
			}
		}
	}
	total := (len(a) - 1) + (len(b) - 1)
	if total == 0 {
		return 0
	}
	return 2.0 * float64(inter) / float64(total)
}
