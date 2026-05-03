package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

	// FTP ADMIN (team-shared, baked au build)
	FTPHost     string `json:"ftp_host"`
	FTPPort     int    `json:"ftp_port"`
	FTPUser     string `json:"ftp_user"`
	FTPPassword string `json:"ftp_password"`
	FTPPath     string `json:"ftp_path"`

	// FTP Privé (perso de chaque user, saisi dans Réglages, jamais baked)
	PrivateFTPHost     string `json:"private_ftp_host"`
	PrivateFTPPort     int    `json:"private_ftp_port"`
	PrivateFTPUser     string `json:"private_ftp_user"`
	PrivateFTPPassword string `json:"private_ftp_password"`
	PrivateFTPPath     string `json:"private_ftp_path"`

	// Seedbox ADMIN (team-shared ruTorrent, baked au build)
	SeedboxURL      string `json:"seedbox_url"`      // ex: https://my-seedbox.example/rutorrent/
	SeedboxUser     string `json:"seedbox_user"`
	SeedboxPassword string `json:"seedbox_password"`
	SeedboxLabel    string `json:"seedbox_label"`    // label optionnel ajouté au torrent

	// Seedbox Privée ruTorrent (perso de chaque user)
	PrivateSeedboxURL      string `json:"private_seedbox_url"`
	PrivateSeedboxUser     string `json:"private_seedbox_user"`
	PrivateSeedboxPassword string `json:"private_seedbox_password"`
	PrivateSeedboxLabel    string `json:"private_seedbox_label"`

	// Seedbox Privée qBittorrent (perso de chaque user, alternative à ruTorrent)
	PrivateQBitURL      string `json:"private_qbit_url"`
	PrivateQBitUser     string `json:"private_qbit_user"`
	PrivateQBitPassword string `json:"private_qbit_password"`

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

	// NextCloud ADMIN — historiquement utilisé pour Torrent ADMIN (revert en v6.0.2,
	// reste en config pour rétrocompat — workflow ADMIN repassé sur FTP+ruTorrent).
	NextcloudAdminURL      string `json:"nextcloud_admin_url"`
	NextcloudAdminUser     string `json:"nextcloud_admin_user"`
	NextcloudAdminPassword string `json:"nextcloud_admin_password"`
	NextcloudAdminPath     string `json:"nextcloud_admin_path"`

	// NextCloud MOD — upload MKV via WebDAV pour le workflow Torrent MODO.
	// La seedbox MOD (cluster1c.seedbox.fr) expose un NextCloud, le qBit MODO
	// récupère le MKV depuis le filesystem partagé une fois uploadé.
	NextcloudModURL      string `json:"nextcloud_mod_url"`
	NextcloudModUser     string `json:"nextcloud_mod_user"`
	NextcloudModPassword string `json:"nextcloud_mod_password"`
	NextcloudModPath     string `json:"nextcloud_mod_path"`

	// qBittorrent ADMIN — remplace ruTorrent ADMIN (cfg.SeedboxURL).
	QBitAdminURL      string `json:"qbit_admin_url"`
	QBitAdminUser     string `json:"qbit_admin_user"`
	QBitAdminPassword string `json:"qbit_admin_password"`

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

	// Credentials TEAM-SHARED bakés au build via secrets GitHub.
	// Chaque user voit ces valeurs au 1er démarrage, overridées à chaque
	// lancement pour rester à jour (si la team change un mdp partagé).
	//
	// NOTE : les champs "Privé" (PrivateFTP*, PrivateSeedbox*) ne sont PAS
	// bakés — chaque user les saisit manuellement dans Réglages pour utiliser
	// sa PROPRE seedbox perso via le bouton "Torrent Privé".

	// FTP ADMIN (team-shared seedbox Gandalf nod47.ma-seedbox.me)
	DefaultFTPHost     = ""
	DefaultFTPUser     = ""
	DefaultFTPPassword = ""
	DefaultFTPPath     = ""

	// Seedbox ADMIN ruTorrent (Gandalf)
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

	// NextCloud ADMIN — team-shared, baké au build via secrets GitHub.
	DefaultNextcloudAdminURL      = ""
	DefaultNextcloudAdminUser     = ""
	DefaultNextcloudAdminPassword = ""
	DefaultNextcloudAdminPath     = ""

	// qBittorrent ADMIN (URL différente du qBit MODO)
	DefaultQBitAdminURL      = ""
	DefaultQBitAdminUser     = ""
	DefaultQBitAdminPassword = ""

	// NextCloud MOD — team-shared, baké au build (workflow Torrent MODO)
	DefaultNextcloudModURL      = ""
	DefaultNextcloudModUser     = ""
	DefaultNextcloudModPassword = ""
	DefaultNextcloudModPath     = ""

	DefaultTrackerURL = ""

	// TMDB : URLs bakées au build pour verrouiller la section TMDB côté user.
	// Le user ne peut pas les modifier (inputs disabled + override à chaque Load).
	DefaultTMDBProxyURL   = ""
	DefaultMediaSearchURL = ""
	DefaultTMDBApiKey     = ""

	// Index de recherche TEAM (dossier LiHDL team-shared, baké au build)
	DefaultLihdlBaseURL = ""
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
		PrivateFTPPort:  21,
		FTPModPort:      21,
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
	// Les secrets GitHub peuvent contenir des whitespaces/newlines parasites
	// à cause du paste/UI — on trim systématiquement pour éviter les
	// "no such host" ou "auth failed" mystérieux.
	override := func(target *string, def string) {
		if t := strings.TrimSpace(def); t != "" {
			*target = t
		}
	}
	override(&cfg.FTPHost, DefaultFTPHost)
	override(&cfg.FTPUser, DefaultFTPUser)
	override(&cfg.FTPPassword, DefaultFTPPassword)
	override(&cfg.FTPPath, DefaultFTPPath)
	override(&cfg.SeedboxURL, DefaultSeedboxURL)
	override(&cfg.SeedboxUser, DefaultSeedboxUser)
	override(&cfg.SeedboxPassword, DefaultSeedboxPassword)
	override(&cfg.SeedboxLabel, DefaultSeedboxLabel)
	override(&cfg.QBitURL, DefaultQBitURL)
	override(&cfg.QBitUser, DefaultQBitUser)
	override(&cfg.QBitPassword, DefaultQBitPassword)
	override(&cfg.FTPModHost, DefaultFTPModHost)
	override(&cfg.FTPModUser, DefaultFTPModUser)
	override(&cfg.FTPModPassword, DefaultFTPModPassword)
	override(&cfg.FTPModPath, DefaultFTPModPath)
	override(&cfg.NextcloudAdminURL, DefaultNextcloudAdminURL)
	override(&cfg.NextcloudAdminUser, DefaultNextcloudAdminUser)
	override(&cfg.NextcloudAdminPassword, DefaultNextcloudAdminPassword)
	override(&cfg.NextcloudAdminPath, DefaultNextcloudAdminPath)
	override(&cfg.QBitAdminURL, DefaultQBitAdminURL)
	override(&cfg.QBitAdminUser, DefaultQBitAdminUser)
	override(&cfg.QBitAdminPassword, DefaultQBitAdminPassword)
	override(&cfg.NextcloudModURL, DefaultNextcloudModURL)
	override(&cfg.NextcloudModUser, DefaultNextcloudModUser)
	override(&cfg.NextcloudModPassword, DefaultNextcloudModPassword)
	override(&cfg.NextcloudModPath, DefaultNextcloudModPath)
	override(&cfg.TrackerURL, DefaultTrackerURL)
	override(&cfg.TMDBProxyURL, DefaultTMDBProxyURL)
	override(&cfg.MediaSearchURL, DefaultMediaSearchURL)
	override(&cfg.TMDBApiKey, DefaultTMDBApiKey)
	override(&cfg.LihdlBaseURL, DefaultLihdlBaseURL)
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
