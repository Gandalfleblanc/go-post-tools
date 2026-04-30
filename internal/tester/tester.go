package tester

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"

	"go-post-tools/internal/nextcloud"
	"go-post-tools/internal/seedbox"
	"go-post-tools/internal/webdav"
)

type Result struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func ok(msg string) Result  { return Result{true, msg} }
func fail(err error) Result { return Result{false, err.Error()} }

func TestLihdl(baseURL, user, password string) Result {
	if baseURL == "" {
		return Result{false, "URL LiHDL non configurée"}
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	req, _ := http.NewRequest("GET", baseURL+"?C=M;O=D", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if user != "" {
		req.SetBasicAuth(user, password)
	}
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return fail(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return ok("Connecté à l'index LiHDL")
	}
	if resp.StatusCode == 401 {
		wa := resp.Header.Get("WWW-Authenticate")
		return Result{false, fmt.Sprintf("Auth refusée (401) — WWW-Authenticate: %s", wa)}
	}
	return Result{false, fmt.Sprintf("HTTP %d", resp.StatusCode)}
}

func TestHydracker(baseURL, token string) Result {
	if baseURL == "" {
		return Result{false, "URL Hydracker non configurée"}
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(baseURL, "/api/v1") {
		baseURL += "/api/v1"
	}
	// /meta/langs : endpoint simple protégé par auth. Distinct des résultats :
	//   200 → token OK
	//   401 → token invalide/expiré
	//   404 → URL de base incorrecte
	req, _ := http.NewRequest("GET", baseURL+"/meta/langs", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "GoPostTools/4.x (https://github.com/Gandalfleblanc/Go-Post-Tools)")
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return fail(err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
		return ok("Connecté à Hydracker ✓")
	case 401:
		return Result{false, "Token invalide ou expiré — vérifie la clé d'accès"}
	case 404:
		return Result{false, fmt.Sprintf("URL de base incorrecte (404) — utilisé : %s", baseURL)}
	default:
		return Result{false, fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
}

// TestTMDB ping le proxytmdb (pas besoin de clé). Le paramètre apiKey est
// gardé pour compatibilité de signature mais ignoré.
func TestTMDB(apiKey string) Result {
	_ = apiKey
	c := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", "https://tmdb.uklm.xyz/health", nil)
	req.Header.Set("User-Agent", "GoPostTools/4.x")
	resp, err := c.Do(req)
	if err != nil {
		return Result{false, fmt.Sprintf("Proxy TMDB injoignable: %v", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return Result{false, fmt.Sprintf("Proxy TMDB HTTP %d", resp.StatusCode)}
	}
	return ok("Proxy TMDB (tmdb.uklm.xyz) OK — pas de clé requise")
}

func TestOneFichier(apiKey string) Result {
	req, _ := http.NewRequest("GET", "https://api.1fichier.com/v1/account/info.cgi", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return fail(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return ok("Connecté à 1Fichier")
	}
	return Result{false, fmt.Sprintf("HTTP %d", resp.StatusCode)}
}

func TestSendCm(apiKey string) Result {
	req, _ := http.NewRequest("GET", "https://send.cm/api/account/info", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return fail(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return ok("Connecté à Send.now")
	}
	return Result{false, fmt.Sprintf("HTTP %d", resp.StatusCode)}
}

func TestFTP(host string, port int, user, password string) Result {
	if host == "" {
		return fail(fmt.Errorf("host manquant"))
	}
	if port <= 0 {
		port = 21
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := ftp.Dial(addr, ftp.DialWithTimeout(10*time.Second))
	if err != nil {
		return fail(err)
	}
	defer conn.Quit()
	if err := conn.Login(user, password); err != nil {
		return fail(err)
	}
	return ok("Connexion FTP réussie")
}

func TestSeedbox(url, user, password string) Result {
	if err := seedbox.Ping(url, user, password); err != nil {
		return fail(err)
	}
	return ok("Connexion ruTorrent réussie")
}

// TestQBit : test de connexion qBittorrent Web UI via POST /api/v2/auth/login.
// qBit renvoie "Ok." en body si login OK, "Fails." sinon.
func TestQBit(baseURL, user, password string) Result {
	if baseURL == "" {
		return fail(fmt.Errorf("URL qBittorrent non configurée"))
	}
	u := strings.TrimRight(baseURL, "/") + "/api/v2/auth/login"
	form := strings.NewReader("username=" + user + "&password=" + password)
	req, _ := http.NewRequest("POST", u, form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", strings.TrimRight(baseURL, "/"))
	req.Header.Set("User-Agent", "GoPostTools/3.3")
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return fail(err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 128)
	n, _ := resp.Body.Read(buf)
	body := strings.TrimSpace(string(buf[:n]))
	if resp.StatusCode != 200 {
		return fail(fmt.Errorf("HTTP %d: %s", resp.StatusCode, body))
	}
	if strings.HasPrefix(body, "Ok") {
		return ok("Connexion qBittorrent réussie")
	}
	return fail(fmt.Errorf("login refusé: %s", body))
}

// TestModSeedbox : PROPFIND sur la racine WebDAV pour valider URL + credentials.
// Compatible Nextcloud / ownCloud (endpoint /remote.php/dav/files/{user}/).
func TestModSeedbox(baseURL, user, password string) Result {
	if err := webdav.Ping(baseURL, user, password); err != nil {
		return fail(err)
	}
	return ok("Connexion WebDAV réussie (Seedbox Modérateur)")
}

// TestNextcloud : PROPFIND sur la racine WebDAV NextCloud (cert self-signed accepté).
func TestNextcloud(baseURL, user, password string) Result {
	if err := nextcloud.Ping(baseURL, user, password); err != nil {
		return fail(err)
	}
	return ok("Connexion NextCloud réussie")
}

func TestUsenet(host string, port int) Result {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fail(err)
	}
	defer conn.Close()
	return ok(fmt.Sprintf("Serveur Usenet joignable (%s)", addr))
}
