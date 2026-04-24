package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	// Hydracker
	HydrackerToken string `json:"hydracker_token"`

	// TMDB (proxy = serveur sans clé requise ; clé TMDB gardée en fallback)
	TMDBApiKey   string `json:"tmdb_api_key"`
	TMDBProxyURL string `json:"tmdb_proxy_url"` // default https://tmdb.uklm.xyz

	// 1Fichier
	OneFichierApiKey string `json:"one_fichier_api_key"`

	// SEND.CM
	SendCmApiKey string `json:"sendcm_api_key"`

	// Nexum (tracker secondaire — clé API personnelle)
	NexumApiKey  string `json:"nexum_api_key"`
	NexumBaseURL string `json:"nexum_base_url"` // ex: https://nexum-core.com

	// Usenet
	UsenetHost     string `json:"usenet_host"`
	UsenetPort     int    `json:"usenet_port"`
	UsenetSSL      bool   `json:"usenet_ssl"`
	UsenetUser     string `json:"usenet_user"`
	UsenetPassword string `json:"usenet_password"`
	UsenetConns    int    `json:"usenet_connections"`
	UsenetGroup    string `json:"usenet_group"`

	// ParPar
	ParParRedundancy float64 `json:"parpar_redundancy"`
	ParParThreads    int     `json:"parpar_threads"`
	ParParSliceSize  int     `json:"parpar_slice_size"`

	// FTP
	FTPHost     string `json:"ftp_host"`
	FTPPort     int    `json:"ftp_port"`
	FTPUser     string `json:"ftp_user"`
	FTPPassword string `json:"ftp_password"`
	FTPPath     string `json:"ftp_path"`

	// Seedbox (ruTorrent Web UI)
	SeedboxURL      string `json:"seedbox_url"`      // ex: https://my-seedbox.example/rutorrent/
	SeedboxUser     string `json:"seedbox_user"`
	SeedboxPassword string `json:"seedbox_password"`
	SeedboxLabel    string `json:"seedbox_label"`    // label optionnel ajouté au torrent

	// Seedbox qBittorrent Web UI (alternative à ruTorrent pour les modérateurs
	// qui utilisent qBit). URL + login/password.
	QBitURL      string `json:"qbit_url"`
	QBitUser     string `json:"qbit_user"`
	QBitPassword string `json:"qbit_password"`

	// Seedbox modérateur — site web avec login/password où les modérateurs
	// uploadent leurs fichiers, qui sont ensuite synchronisés avec la seedbox.
	ModSeedboxURL      string `json:"mod_seedbox_url"`
	ModSeedboxUser     string `json:"mod_seedbox_user"`
	ModSeedboxPassword string `json:"mod_seedbox_password"`

	// FTP modérateur (ex: seedbox.fr bloque WebDAV chunked → FTP est la
	// méthode standard pour les gros fichiers).
	FTPModHost     string `json:"ftp_mod_host"`
	FTPModPort     int    `json:"ftp_mod_port"`
	FTPModUser     string `json:"ftp_mod_user"`
	FTPModPassword string `json:"ftp_mod_password"`
	FTPModPath     string `json:"ftp_mod_path"`

	// Hash sha256 du mot de passe qui protège les 4 sections seedbox/FTP
	// (FTP RUTORRENT, FTP MODÉRATEUR, Seedbox ruTorrent, Seedbox qBit).
	// Mutualisé pour 1 mdp partagé entre admins.
	SeedboxSettingsPasswordHash string `json:"seedbox_settings_password_hash"`

	// Flag définitif : une fois que l'user a entré le mdp partagé pour
	// débloquer l'option "Torrent ADMIN", on sauvegarde true ici et on
	// ne redemande plus jamais. Un vrai user peut toujours modifier ce
	// flag manuellement dans config.json pour re-demander (reset).
	TorrentAdminAcknowledged bool `json:"torrent_admin_acknowledged"`

	// Torrent creation
	TrackerURL       string `json:"tracker_url"`
	TorrentPieceSize int    `json:"torrent_piece_size"` // en octets

	// Hydracker — URL de base de l'API (à configurer par l'utilisateur)
	HydrackerBaseURL string `json:"hydracker_base_url"` // ex: https://exemple.tld

	// LiHDL index + endpoint recherche TMDB (URLs + Basic Auth, à configurer)
	LihdlBaseURL   string `json:"lihdl_base_url"`     // ex: https://exemple.tld/chemin/LiHDL/
	MediaSearchURL string `json:"media_search_url"`   // ex: https://exemple.tld/search.php?query=
	LihdlUser      string `json:"lihdl_user"`
	LihdlPassword  string `json:"lihdl_password"`
	// Hash sha256 du mot de passe qui protège la section LiHDL dans les Réglages UI.
	LihdlSettingsPasswordHash string `json:"lihdl_settings_password_hash"`

	// Watch folder
	WatchFolder    string `json:"watch_folder"`
	WatchAutoStart bool   `json:"watch_auto_start"`

	// Proxy HTTP/HTTPS appliqué à tous les clients HTTP (api.Client, TMDB,
	// 1fichier, GitHub update, etc.). Format supporté : http://host:port,
	// http://user:pass@host:port, socks5://host:port. Laissé vide = pas de proxy.
	// Pris en compte via HTTP_PROXY/HTTPS_PROXY env vars (setés au startup et
	// à chaque SaveConfig) — Go's http.DefaultTransport lit ces vars à chaque
	// requête via http.ProxyFromEnvironment.
	ProxyURL string `json:"proxy_url"`
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "go-post-tools", "config.json")
}

// Defaults injectés au build via ldflags (-X go-post-tools/internal/config.DefaultXxx=...).
// Seul le hash de déverrouillage de la section LiHDL est injecté — les URLs restent vides
// par défaut, l'utilisateur les saisit manuellement dans Réglages.
var (
	// DefaultLihdlUnlockHash : hash SHA-256 du mot de passe de déverrouillage de la section LiHDL.
	// Injecté au build depuis le secret GitHub LIHDL_UNLOCK_HASH. Non modifiable par l'utilisateur.
	DefaultLihdlUnlockHash = ""
	// DefaultHydrackerBaseURL : URL de base de l'API Hydracker.
	// Injectée au build depuis le secret GitHub HYDRACKER_URL. Si non-vide, verrouille
	// l'URL côté utilisateur (pas modifiable dans Réglages).
	DefaultHydrackerBaseURL = ""
	// DefaultSeedboxUnlockHash : hash SHA-256 du mot de passe de déverrouillage
	// des sections Seedbox / FTP / Tracker Torrent. Injecté depuis MODO_UNLOCK_HASH.
	// Si non-vide, les admins update auto avec la protection activée, mdp partagé.
	DefaultSeedboxUnlockHash = ""

	// Credentials baked au build via secrets GitHub — pre-remplis au 1er démarrage
	// si config.json n'a rien pour ce champ.
	DefaultFTPHost     = ""
	DefaultFTPUser     = ""
	DefaultFTPPassword = ""
	DefaultFTPPath     = ""

	DefaultSeedboxURL      = ""
	DefaultSeedboxUser     = ""
	DefaultSeedboxPassword = ""
	DefaultSeedboxLabel    = ""

	DefaultQBitURL      = ""
	DefaultQBitUser     = ""
	DefaultQBitPassword = ""

	DefaultFTPModHost     = ""
	DefaultFTPModUser     = ""
	DefaultFTPModPassword = ""
	DefaultFTPModPath     = ""

	DefaultTrackerURL = ""

	// TMDB : URLs bakées au build pour verrouiller la section TMDB côté user.
	// Le user ne peut pas les modifier (inputs disabled + override à chaque Load).
	DefaultTMDBProxyURL   = ""
	DefaultMediaSearchURL = ""
	DefaultTMDBApiKey     = ""
)

func Load() *Config {
	cfg := &Config{
		UsenetPort:      119,
		UsenetConns:     20,
		UsenetGroup:     "alt.binaries.test",
		ParParRedundancy: 5,
		ParParThreads:   8,
		ParParSliceSize: 768000,
		FTPPort:         21,
		TorrentPieceSize: 8 * 1024 * 1024, // 8 MiB
		NexumBaseURL:    "https://nexum-core.com",
		TMDBProxyURL:    "https://tmdb.uklm.xyz",
		WatchFolder:     filepath.Join(func() string { h, _ := os.UserHomeDir(); return h }(), "Desktop", "LiHDL"),
	}
	data, err := os.ReadFile(configPath())
	if err == nil {
		_ = json.Unmarshal(data, cfg)
	}
	// Override avec les defaults bakés au build SI ils sont définis (non-vides).
	// Sinon préserve la valeur user. Comme ça :
	//  - si la team déploie une version avec creds bakés → tout le monde
	//    a les bonnes valeurs au démarrage (override les user qui auraient
	//    saisi/conservé une vieille valeur fausse)
	//  - si build sans secret (dev local par ex) → chaque user garde son perso
	if DefaultFTPHost != "" {
		cfg.FTPHost = DefaultFTPHost
	}
	if DefaultFTPUser != "" {
		cfg.FTPUser = DefaultFTPUser
	}
	if DefaultFTPPassword != "" {
		cfg.FTPPassword = DefaultFTPPassword
	}
	if DefaultFTPPath != "" {
		cfg.FTPPath = DefaultFTPPath
	}
	if DefaultSeedboxURL != "" {
		cfg.SeedboxURL = DefaultSeedboxURL
	}
	if DefaultSeedboxUser != "" {
		cfg.SeedboxUser = DefaultSeedboxUser
	}
	if DefaultSeedboxPassword != "" {
		cfg.SeedboxPassword = DefaultSeedboxPassword
	}
	if DefaultSeedboxLabel != "" {
		cfg.SeedboxLabel = DefaultSeedboxLabel
	}
	if DefaultQBitURL != "" {
		cfg.QBitURL = DefaultQBitURL
	}
	if DefaultQBitUser != "" {
		cfg.QBitUser = DefaultQBitUser
	}
	if DefaultQBitPassword != "" {
		cfg.QBitPassword = DefaultQBitPassword
	}
	if DefaultFTPModHost != "" {
		cfg.FTPModHost = DefaultFTPModHost
	}
	if DefaultFTPModUser != "" {
		cfg.FTPModUser = DefaultFTPModUser
	}
	if DefaultFTPModPassword != "" {
		cfg.FTPModPassword = DefaultFTPModPassword
	}
	if DefaultFTPModPath != "" {
		cfg.FTPModPath = DefaultFTPModPath
	}
	if DefaultTrackerURL != "" {
		cfg.TrackerURL = DefaultTrackerURL
	}
	if DefaultTMDBProxyURL != "" {
		cfg.TMDBProxyURL = DefaultTMDBProxyURL
	}
	if DefaultMediaSearchURL != "" {
		cfg.MediaSearchURL = DefaultMediaSearchURL
	}
	if DefaultTMDBApiKey != "" {
		cfg.TMDBApiKey = DefaultTMDBApiKey
	}
	return cfg
}

func Save(cfg *Config) error {
	path := configPath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
