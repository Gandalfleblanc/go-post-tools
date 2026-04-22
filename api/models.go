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
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Poster      string  `json:"poster,omitempty"`
	ReleaseDate string  `json:"release_date,omitempty"`
	Score       float64 `json:"score,omitempty"`
	Runtime     int     `json:"runtime,omitempty"`
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

// --- Content (paid) ---

type Lien struct {
	ID       int    `json:"id"`
	URL      string `json:"lien"`
	Lang     int    `json:"lang_id,omitempty"`
	Quality  int    `json:"qual_id,omitempty"`
	Episode  int    `json:"episode,omitempty"`
	Season   int    `json:"saison,omitempty"`
	Host     string `json:"host,omitempty"`
	Uploader string `json:"id_user,omitempty"`
}

type Nzb struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
	Lang        int    `json:"lang_id,omitempty"`
	Quality     int    `json:"qual_id,omitempty"`
	Episode     int    `json:"episode,omitempty"`
	Season      int    `json:"saison,omitempty"`
	Uploader    string `json:"id_user,omitempty"`
}

type TorrentItem struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
	Lang        int    `json:"lang_id,omitempty"`
	Quality     int    `json:"qual_id,omitempty"`
	Episode     int    `json:"episode,omitempty"`
	Season      int    `json:"saison,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Uploader    string `json:"id_user,omitempty"`
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
