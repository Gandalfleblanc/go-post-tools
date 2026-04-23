package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	// Hydracker
	HydrackerToken string `json:"hydracker_token"`

	// TMDB
	TMDBApiKey string `json:"tmdb_api_key"`

	// 1Fichier
	OneFichierApiKey string `json:"one_fichier_api_key"`

	// SEND.CM
	SendCmApiKey string `json:"sendcm_api_key"`

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
		WatchFolder:     filepath.Join(func() string { h, _ := os.UserHomeDir(); return h }(), "Desktop", "LiHDL"),
	}
	data, err := os.ReadFile(configPath())
	if err == nil {
		_ = json.Unmarshal(data, cfg)
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
