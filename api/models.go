package api

// --- Pagination ---

type Pagination struct {
	CurrentPage int `json:"current_page"`
	PerPage     int `json:"per_page"`
	Total       int `json:"total"`
	LastPage    int `json:"last_page"`
}

// --- Auth ---

type LoginRequest struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	TokenName string `json:"token_name,omitempty"`
	Token     string `json:"token,omitempty"`
}

type RegisterRequest struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	TokenName string `json:"token_name,omitempty"`
}

// --- User ---

type User struct {
	ID            int     `json:"id"`
	Username      string  `json:"username"`
	Email         string  `json:"email,omitempty"`
	AccessToken   string  `json:"access_token,omitempty"`
	Avatar        string  `json:"avatar,omitempty"`
	Image         string  `json:"image,omitempty"`
	CreatedAt     string  `json:"created_at,omitempty"`
	IsPremium     bool    `json:"IsPremium,omitempty"`
	IsPro         bool    `json:"is_pro,omitempty"`
	WalletBalance string  `json:"wallet_balance,omitempty"`
	Uploaded      int64   `json:"uploaded,omitempty"`
	Downloaded    int64   `json:"downloaded,omitempty"`
	Ratio         string  `json:"ratio,omitempty"`
	Followers     int     `json:"followers_count,omitempty"`
	Following     int     `json:"followed_users_count,omitempty"`
	ListsCount    int     `json:"lists_count,omitempty"`
	APIEnabled    bool    `json:"api_content_enabled,omitempty"`
	Language      string  `json:"language,omitempty"`
	Country       string  `json:"country,omitempty"`
	Bio           string  `json:"bio,omitempty"`
	Status        string  `json:"status,omitempty"`
	PremiumExpire string  `json:"premium_expire,omitempty"`
	UnlimitedUntil *string `json:"unlimited_until,omitempty"`
}

// --- Titles ---

type PartialTitle struct {
	ID                 int     `json:"id"`
	Name               string  `json:"name"`
	Type               string  `json:"type"`
	Poster             string  `json:"poster,omitempty"`
	ReleaseDate        string  `json:"release_date,omitempty"`
	Score              float64 `json:"score,omitempty"`
	Runtime            int     `json:"runtime,omitempty"`
	LastContentAddedAt string  `json:"last_content_added_at,omitempty"`
}

type FullTitle struct {
	PartialTitle
	Description string         `json:"description,omitempty"`
	Budget      int64          `json:"budget,omitempty"`
	Revenue     int64          `json:"revenue,omitempty"`
	Language    string         `json:"language,omitempty"`
	Country     string         `json:"country,omitempty"`
	Genres      []Genre        `json:"genres,omitempty"`
	Credits     []Credit       `json:"credits,omitempty"`
	Seasons     []Season       `json:"seasons,omitempty"`
}

type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Credit struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Character  string `json:"character,omitempty"`
	Department string `json:"department,omitempty"`
}

type Season struct {
	Number   int       `json:"season_number"`
	Episodes []Episode `json:"episodes,omitempty"`
}

type Episode struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	EpisodeNum   int     `json:"episode_number"`
	SeasonNum    int     `json:"season_number"`
	Overview     string  `json:"overview,omitempty"`
	AirDate      string  `json:"air_date,omitempty"`
	Score        float64 `json:"score,omitempty"`
}

// --- People ---

type PartialPerson struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Popularity float64 `json:"popularity,omitempty"`
	Avatar     string `json:"avatar,omitempty"`
}

type FullPerson struct {
	PartialPerson
	Biography   string         `json:"biography,omitempty"`
	BirthDate   string         `json:"birth_date,omitempty"`
	DeathDate   string         `json:"death_date,omitempty"`
	BirthPlace  string         `json:"birth_place,omitempty"`
	Credits     []PartialTitle `json:"credits,omitempty"`
}

// --- News ---

type NewsArticle struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// --- Lists ---

type PartialList struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Public      bool   `json:"public"`
	UserID      int    `json:"user_id"`
}

type FullList struct {
	PartialList
	Items []ListItem `json:"items,omitempty"`
}

type ListItem struct {
	ID       int    `json:"id"`
	ItemType string `json:"item_type"`
}

type CrupdateListPayload struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Public      bool   `json:"public,omitempty"`
}

type ListItemPayload struct {
	ItemID   int    `json:"itemId"`
	ItemType string `json:"itemType"` // title | person | episode
}

// --- Reviews ---

type Review struct {
	ID        int    `json:"id"`
	Score     int    `json:"score"`
	Body      string `json:"body,omitempty"`
	UserID    int    `json:"user_id"`
	TitleID   int    `json:"title_id"`
	CreatedAt string `json:"created_at,omitempty"`
}

type CreateReviewPayload struct {
	TitleID  int    `json:"title_id"`
	Score    int    `json:"score"`
	Review   string `json:"review,omitempty"`
}

type UpdateReviewPayload struct {
	Score  int    `json:"score,omitempty"`
	Review string `json:"review,omitempty"`
}

// --- Content (free via API selon doc) ---
// Note : les noms de champs correspondent à la vraie réponse API
// (observée en prod : qualite, taille, id_host, torrent_name, langues_compact…)
// au lieu des noms qu'on pourrait attendre (qual_id, size, host, name, lang_id).

// LangPivot est une entrée dans langues_compact / subs_compact
// (lang/sub rattachée à un lien ou torrent).
type LangPivot struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// firstLangID extrait le premier lang ID d'un slice langues_compact (0 si vide).
func firstLangID(langs []LangPivot) int {
	if len(langs) == 0 {
		return 0
	}
	return langs[0].ID
}

// PrimaryLangID méthode helper pour récupérer la langue principale
// (premier élément de langues_compact) — remplace l'ancien champ Lang direct.
func (l *Lien) PrimaryLangID() int      { return firstLangID(l.Langues) }
func (t *TorrentItem) PrimaryLangID() int { return firstLangID(t.Langues) }
func (n *Nzb) PrimaryLangID() int       { return firstLangID(n.Langues) }

// HostName retourne le nom du host d'un Lien (via Host.Name ou fallback).
func (l *Lien) HostName() string {
	if l.Host != nil {
		return l.Host.Name
	}
	return ""
}

// QualDetails est l'objet qual imbriqué dans un Lien (/admin/liens).
type QualDetails struct {
	IDQual int    `json:"id_qual"`
	Qual   string `json:"qual"`
	Label  string `json:"label"`
}

// HostDetails est l'objet host imbriqué dans un Lien admin.
type HostDetails struct {
	IDHost int      `json:"id_host"`
	Name   string   `json:"name"`
	URL    string   `json:"url,omitempty"`
	Icon   string   `json:"icon,omitempty"`
}

type Lien struct {
	ID          int          `json:"id"`
	TitleID     int          `json:"title_id,omitempty"`
	URL         string       `json:"lien,omitempty"`              // présent dans /content/liens/{id}, absent dans les listes
	IDHost      int          `json:"id_host,omitempty"`
	Quality     int          `json:"qualite,omitempty"`           // API: "qualite" (pas "qual_id")
	Season      int          `json:"saison,omitempty"`
	Episode     int          `json:"episode,omitempty"`
	FullSaison  int          `json:"full_saison,omitempty"`
	Size        int64        `json:"taille,omitempty"`            // API: "taille" (pas "size")
	IDUser      string       `json:"id_user,omitempty"`           // pseudo, pas ID numérique
	Active      int          `json:"active,omitempty"`
	View        int          `json:"view,omitempty"`
	CreatedAt   string       `json:"created_at,omitempty"`
	UpdatedAt   string       `json:"updated_at,omitempty"`
	Qual        *QualDetails `json:"qual,omitempty"`              // objet qual imbriqué
	Host        *HostDetails `json:"host,omitempty"`              // objet host (présent dans /admin/liens)
	Langues     []LangPivot  `json:"langues_compact,omitempty"`   // au lieu d'un simple lang_id
	Subs        []LangPivot  `json:"subs_compact,omitempty"`
}

type Nzb struct {
	ID          int         `json:"id"`
	TitleID     int         `json:"title_id,omitempty"`
	Name        string      `json:"name,omitempty"`
	DownloadURL string      `json:"download_url,omitempty"`
	Quality     int         `json:"qualite,omitempty"`
	Season      int         `json:"saison,omitempty"`
	Episode     int         `json:"episode,omitempty"`
	Size        int64       `json:"size,omitempty"` // NZB API renvoie "size" (pas "taille")
	IDUser      string      `json:"id_user,omitempty"`
	Author      string      `json:"author,omitempty"` // /content/nzbs renvoie author, /admin/nzb aussi
	Active      int         `json:"active,omitempty"`
	CreatedAt   string      `json:"created_at,omitempty"`
	UpdatedAt   string      `json:"updated_at,omitempty"`
	Qual        *QualDetails `json:"qual,omitempty"`
	Langues     []LangPivot `json:"langues_compact,omitempty"`
	Subs        []LangPivot `json:"subs_compact,omitempty"`
}

type TorrentItem struct {
	ID          int         `json:"id"`
	TitleID     int         `json:"title_id,omitempty"`
	Name        string      `json:"torrent_name,omitempty"`      // API: "torrent_name" (pas "name")
	DownloadURL string      `json:"download_url,omitempty"`
	InfoHash    string      `json:"info_hash,omitempty"`
	Hash        string      `json:"hash,omitempty"`
	Quality     int         `json:"qualite,omitempty"`
	Season      int         `json:"saison,omitempty"`
	Episode     int         `json:"episode,omitempty"`
	FullSaison  bool        `json:"full_saison,omitempty"`
	Size        int64       `json:"taille,omitempty"`
	Seeders     int         `json:"seeders,omitempty"`
	Leechers    int         `json:"leechers,omitempty"`
	Completed   int         `json:"completed,omitempty"`
	Author      string      `json:"author,omitempty"`            // API: "author" (pas "id_user")
	Active      bool        `json:"active,omitempty"`            // /content/torrents/{id} renvoie bool (pas int)
	CreatedAt   string      `json:"created_at,omitempty"`
	UpdatedAt   string      `json:"updated_at,omitempty"`
	Qual        *QualDetails `json:"qual,omitempty"`
	Langues     []LangPivot `json:"langues_compact,omitempty"`
	Subs        []LangPivot `json:"subs_compact,omitempty"`
}

type ContentResult[T any] struct {
	Items      []T     `json:"-"`
	Count      int     `json:"count"`
	Charged    float64 `json:"charged"`
	AlreadyPaid int    `json:"already_paid"`
}

// --- Meta ---

type Lang struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Code string `json:"code,omitempty"`
}

type Quality struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// --- Upload ---

type UploadLienPayload struct {
	TitleID  int    `json:"title_id"`
	Lien     string `json:"lien"`
	Quality  int    `json:"qualite,omitempty"`
	Langues  []int  `json:"langues,omitempty"`
	Subs     []int  `json:"subs,omitempty"`
	Episode  int    `json:"episode,omitempty"`
	Season   int    `json:"saison,omitempty"`
}

// --- Filters ---

type TitleFilter struct {
	PerPage       int
	Page          int
	Order         string
	Type          string
	Genre         string
	Released      string
	Runtime       string
	Score         string
	Language      string
	Certification string
	Country       string
	OnlyStreamable bool
	IncludeAdult  bool
	TmdbID        int
	ImdbID        string
}

type ContentFilter struct {
	Lang    int
	Quality int
	Episode int
	Season  int
	Limit   int
}
