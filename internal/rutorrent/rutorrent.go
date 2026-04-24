package rutorrent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Torrent struct {
	Hash     string `json:"hash"`
	Name     string `json:"name"`      // nom d'affichage (release)
	FileName string `json:"file_name"` // nom du fichier principal (mkv)
	State    int    `json:"state"`     // 0 stopped, 1 started
	IsActive int    `json:"is_active"` // 0 inactive, 1 active
	Size     int64  `json:"size"`
	Done     int64  `json:"done"`
	Message  string `json:"message"` // message d'erreur rtorrent
	HasError bool   `json:"has_error"`
}

// List renvoie tous les torrents via httprpc mode=list.
// Le body retourne un tableau JSON imbriqué dont l'index est le hash.
func List(baseURL, user, password string) ([]Torrent, error) {
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	endpoint := baseURL + "plugins/httprpc/action.php"

	form := url.Values{}
	form.Set("mode", "list")

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if user != "" {
		req.SetBasicAuth(user, password)
	}

	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connexion httprpc: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("authentification refusée (401)")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Format httprpc: {"t": {"<hash>": ["is_open","is_hash_checking",...]}, ...}
	var outer struct {
		Torrents map[string][]interface{} `json:"t"`
	}
	if err := json.Unmarshal(body, &outer); err != nil {
		return nil, fmt.Errorf("parse httprpc: %w (body: %s)", err, truncate(string(body), 200))
	}

	out := make([]Torrent, 0, len(outer.Torrents))
	for hash, arr := range outer.Torrents {
		t := Torrent{Hash: hash}
		// Indexes httprpc standard : https://github.com/Novik/ruTorrent/wiki/PluginHttpRPC
		// 0 is_open, 1 is_hash_checking, 2 is_hash_checked, 3 get_state, 4 get_name,
		// 5 get_size_bytes, 6 get_completed_chunks, 7 get_size_chunks, 8 get_bytes_done,
		// 9 get_up_total, 10 get_ratio, 11 get_up_rate, 12 get_down_rate, 13 get_chunk_size,
		// 14 get_custom1 (label), 15 get_peers_accounted, 16 get_peers_not_connected,
		// 17 get_peers_connected, 18 get_peers_complete, 19 get_left_bytes,
		// 20 get_priority, 21 state_changed, 22 skip_total, 23 hashing,
		// 24 get_chunks_hashed, 25 get_base_path, 26 get_creation_date,
		// 27 get_tracker_focus, 28 is_active, 29 get_message, 30 get_custom2,
		// 31 get_free_diskspace, 32 is_private, 33 is_multi_file
		if v, ok := arr[3].(string); ok {
			t.State = atoi(v)
		}
		if v, ok := arr[4].(string); ok {
			t.Name = v
		}
		if v, ok := arr[5].(string); ok {
			t.Size = atoi64(v)
		}
		if v, ok := arr[8].(string); ok {
			t.Done = atoi64(v)
		}
		if len(arr) > 25 {
			if v, ok := arr[25].(string); ok {
				t.FileName = v
			}
		}
		if len(arr) > 28 {
			if v, ok := arr[28].(string); ok {
				t.IsActive = atoi(v)
			}
		}
		if len(arr) > 29 {
			if v, ok := arr[29].(string); ok {
				t.Message = v
				if v != "" {
					t.HasError = true
				}
			}
		}
		out = append(out, t)
	}
	return out, nil
}

// Recheck force une vérification complète via XML-RPC : stop → close → check_hash → start.
func Recheck(baseURL, user, password, hash string) error {
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	endpoint := baseURL + "plugins/httprpc/action.php"

	for _, method := range []string{"d.stop", "d.close", "d.check_hash", "d.start"} {
		payload := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><methodCall><methodName>%s</methodName><params><param><value><string>%s</string></value></param></params></methodCall>`, method, hash)

		req, err := http.NewRequest("POST", endpoint, strings.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "text/xml; charset=UTF-8")
		if user != "" {
			req.SetBasicAuth(user, password)
		}
		c := &http.Client{Timeout: 30 * time.Second}
		resp, err := c.Do(req)
		if err != nil {
			return fmt.Errorf("%s: %w", method, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Printf("[Recheck] %s → HTTP %d, body: %s\n", method, resp.StatusCode, truncate(string(body), 300))
		if resp.StatusCode != 200 {
			return fmt.Errorf("%s HTTP %d: %s", method, resp.StatusCode, string(body))
		}
		if strings.Contains(string(body), "<fault>") {
			return fmt.Errorf("%s XML-RPC fault: %s", method, truncate(string(body), 200))
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

// Erase supprime un torrent de rTorrent (stops + d.erase). N'efface PAS les
// fichiers sur disque (rTorrent ne le fait pas par défaut). Le caller doit
// gérer la suppression FTP séparément.
func Erase(baseURL, user, password, hash string) error {
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	endpoint := baseURL + "plugins/httprpc/action.php"
	for _, method := range []string{"d.stop", "d.close", "d.erase"} {
		payload := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><methodCall><methodName>%s</methodName><params><param><value><string>%s</string></value></param></params></methodCall>`, method, hash)
		req, err := http.NewRequest("POST", endpoint, strings.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "text/xml; charset=UTF-8")
		if user != "" {
			req.SetBasicAuth(user, password)
		}
		c := &http.Client{Timeout: 30 * time.Second}
		resp, err := c.Do(req)
		if err != nil {
			return fmt.Errorf("%s: %w", method, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("%s HTTP %d: %s", method, resp.StatusCode, string(body))
		}
		// d.erase peut renvoyer une fault si le torrent n'existe pas — on
		// considère ça comme succès (idempotent).
		if strings.Contains(string(body), "<fault>") && method != "d.erase" {
			return fmt.Errorf("%s XML-RPC fault: %s", method, truncate(string(body), 200))
		}
		time.Sleep(300 * time.Millisecond)
	}
	return nil
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}
func atoi64(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int64(c-'0')
	}
	return n
}
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
