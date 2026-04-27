package parser

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

type FileInfo struct {
	Title    string `json:"title"`
	Year     string `json:"year"`
	Quality  string `json:"quality"`
	Source   string `json:"source"`
	VideoCodec string `json:"video_codec"`
	AudioCodec string `json:"audio_codec"`
	Languages []string `json:"languages"`
	Group    string `json:"group"`
	Season   int    `json:"season"`
	Episode  int    `json:"episode"`
	Raw      string `json:"raw"`
}

var (
	reYear     = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	reQuality  = regexp.MustCompile(`(?i)\b(4K|2160p|1080p|720p|576p|480p)\b`)
	reSource   = regexp.MustCompile(`(?i)\b(BluRay|Blu-Ray|BDRip|BDRemux|HDLight|WEBRip|WEB-DL|WEBDL|HDTV|DVDRip|DVD)\b`)
	reVideo    = regexp.MustCompile(`(?i)\b(x265|x264|H\.?265|H\.?264|HEVC|AVC|XviD|DivX)\b`)
	reAudio    = regexp.MustCompile(`(?i)\b(DTS[-.]?HD|DTS|TrueHD|Atmos|AC3|AAC|MP3|FLAC|DD5\.1|DD2\.0|E-AC3)\b`)
	reLang     = regexp.MustCompile(`(?i)\b(FRENCH|TRUEFRENCH|TrueFrench|VF|VFF|VOF|VOSTFR|MULTI|MULTi|ENGLISH|ENG)\b`)
	reGroup    = regexp.MustCompile(`-([A-Za-z0-9]+)$`)
	reSeparator = regexp.MustCompile(`[._\s]+`)
	// Épisodes : S01E02, S1E2, 1x01, Saison 01 Episode 02
	// Episode jusqu'à 9999 (séries quotidiennes type "Demain nous appartient" qui dépassent 1000)
	reSeasonEpisode = regexp.MustCompile(`(?i)\b[sS](\d{1,2})[\s._-]?[eE](\d{1,4})\b`)
	reCrossEp       = regexp.MustCompile(`\b(\d{1,2})x(\d{1,4})\b`)
	reSeasonOnly    = regexp.MustCompile(`(?i)\b[sS](?:aison)?[\s._-]?(\d{1,2})\b`)
)

func ParseFilename(filename string) *FileInfo {
	name := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	info := &FileInfo{Raw: name}

	// Group (last -GROUP)
	if m := reGroup.FindStringSubmatch(name); m != nil {
		info.Group = m[1]
		name = name[:len(name)-len(m[0])]
	}

	// Year
	if m := reYear.FindString(name); m != "" {
		info.Year = m
	}

	// Quality
	if m := reQuality.FindString(name); m != "" {
		info.Quality = strings.ToLower(m)
		if info.Quality == "2160p" {
			info.Quality = "4k"
		}
	}

	// Source
	if m := reSource.FindString(name); m != "" {
		info.Source = normalizeSource(m)
	}

	// Video codec
	if m := reVideo.FindString(name); m != "" {
		info.VideoCodec = normalizeVideo(m)
	}

	// Audio codec
	if m := reAudio.FindString(name); m != "" {
		info.AudioCodec = normalizeAudio(m)
	}

	// Season / Episode
	if m := reSeasonEpisode.FindStringSubmatch(name); m != nil {
		info.Season = atoiSafe(m[1])
		info.Episode = atoiSafe(m[2])
	} else if m := reCrossEp.FindStringSubmatch(name); m != nil {
		info.Season = atoiSafe(m[1])
		info.Episode = atoiSafe(m[2])
	} else if m := reSeasonOnly.FindStringSubmatch(name); m != nil {
		info.Season = atoiSafe(m[1])
	}

	// Languages
	langs := reLang.FindAllString(name, -1)
	seen := map[string]bool{}
	for _, l := range langs {
		n := normalizeLang(l)
		if !seen[n] {
			seen[n] = true
			info.Languages = append(info.Languages, n)
		}
	}

	// Title: everything before year (or first technical tag)
	title := extractTitle(name)
	info.Title = title

	return info
}

func extractTitle(name string) string {
	// Find the earliest position of year or technical tag
	stopPatterns := []*regexp.Regexp{
		reYear, reQuality, reSource, reVideo, reAudio, reLang,
		reSeasonEpisode, reCrossEp, reSeasonOnly,
	}
	end := len(name)
	for _, pat := range stopPatterns {
		if loc := pat.FindStringIndex(name); loc != nil && loc[0] < end {
			end = loc[0]
		}
	}
	title := name[:end]
	// Replace separators with spaces and clean up
	title = reSeparator.ReplaceAllString(title, " ")
	return strings.TrimSpace(title)
}

func normalizeSource(s string) string {
	s = strings.ToUpper(s)
	switch {
	case strings.Contains(s, "BLURAY") || strings.Contains(s, "BLU-RAY"):
		return "BluRay"
	case strings.Contains(s, "BDRIP"):
		return "BDRip"
	case strings.Contains(s, "BDREMUX"):
		return "BDRemux"
	case strings.Contains(s, "HDLIGHT"):
		return "HDLight"
	case strings.Contains(s, "WEBRIP"):
		return "WEBRip"
	case strings.Contains(s, "WEB-DL"), strings.Contains(s, "WEBDL"):
		return "WEB-DL"
	case strings.Contains(s, "HDTV"):
		return "HDTV"
	case strings.Contains(s, "DVDRIP"):
		return "DVDRip"
	default:
		return s
	}
}

func normalizeVideo(s string) string {
	s = strings.ToUpper(strings.ReplaceAll(s, ".", ""))
	switch {
	case s == "X265" || s == "H265" || s == "HEVC":
		return "H.265"
	case s == "X264" || s == "H264" || s == "AVC":
		return "H.264"
	default:
		return s
	}
}

func normalizeAudio(s string) string {
	u := strings.ToUpper(s)
	switch {
	case strings.Contains(u, "DTSHD"):
		return "DTS-HD"
	case strings.Contains(u, "TRUEHD"):
		return "TrueHD"
	case strings.Contains(u, "ATMOS"):
		return "Atmos"
	case strings.Contains(u, "EAC3") || strings.Contains(u, "E-AC3"):
		return "E-AC3"
	case strings.Contains(u, "AC3"):
		return "AC3"
	case strings.Contains(u, "DTS"):
		return "DTS"
	case strings.Contains(u, "AAC"):
		return "AAC"
	default:
		return s
	}
}

func normalizeLang(s string) string {
	switch strings.ToUpper(s) {
	case "FRENCH", "VF", "VFF":
		return "French"
	case "TRUEFRENCH", "TRUFRENCH":
		return "TrueFrench"
	case "VOF":
		return "VOF"
	case "VOSTFR":
		return "VOSTFR"
	case "MULTI", "MULTII":
		return "Multi"
	case "ENGLISH", "ENG":
		return "English"
	default:
		return s
	}
}
