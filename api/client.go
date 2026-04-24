package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// APILogEntry représente une requête HTTP API avec sa réponse (ou son erreur).
// Utilisé par le hook OnRequestLog pour l'onglet "Log API" qui permet de
// déboguer les appels en temps réel.
type APILogEntry struct {
	Timestamp   string  `json:"ts"`
	Method      string  `json:"method"`
	URL         string  `json:"url"`
	Status      int     `json:"status"`
	DurationMs  int64   `json:"duration_ms"`
	BodyPreview string  `json:"body_preview"`
	Error       string  `json:"error,omitempty"`
}

type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
	ctxMu      sync.Mutex
	ctx        context.Context

	// Rate limit Hydracker = 1 req/s par token (hard cap, doc API).
	// On serialize tous les appels et on garantit min 1.05s entre 2 requêtes.
	rateMu       sync.Mutex
	lastReqAt    time.Time

	// OnRequestLog est appelé à la fin de chaque requête API (succès ou erreur).
	// Permet à App de router les logs vers Wails events pour l'onglet Log API.
	OnRequestLog func(entry APILogEntry)
}

// minInterval respecte le rate limit Hydracker (1 req/s par token).
const minInterval = 1050 * time.Millisecond

// waitRateLimit doit être appelé avant chaque requête. Bloque le temps
// nécessaire pour ne pas dépasser 1 req/s.
func (c *Client) waitRateLimit() {
	c.rateMu.Lock()
	defer c.rateMu.Unlock()
	if !c.lastReqAt.IsZero() {
		elapsed := time.Since(c.lastReqAt)
		if elapsed < minInterval {
			time.Sleep(minInterval - elapsed)
		}
	}
	c.lastReqAt = time.Now()
}

func NewClient(token, baseURL string) *Client {
	return &Client{
		token:      token,
		baseURL:    normalizeBaseURL(baseURL),
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

// normalizeBaseURL garantit que le baseURL se termine par /api/v1 (sans slash final).
func normalizeBaseURL(u string) string {
	u = strings.TrimRight(u, "/")
	if u == "" {
		return ""
	}
	if !strings.HasSuffix(u, "/api/v1") {
		u += "/api/v1"
	}
	return u
}

// BaseURL retourne le baseURL API courant (ex: https://hydracker.com/api/v1).
func (c *Client) BaseURL() string { return c.baseURL }

// SiteURL retourne le domaine sans /api/v1 (ex: https://hydracker.com) pour liens UI.
func (c *Client) SiteURL() string {
	return strings.TrimSuffix(c.baseURL, "/api/v1")
}

func (c *Client) SetToken(token string) {
	c.token = token
}

// SetBaseURL met à jour le baseURL à chaud (ex: changement dans Settings).
func (c *Client) SetBaseURL(u string) {
	c.baseURL = normalizeBaseURL(u)
}

// SetContext définit le contexte utilisé pour les prochaines requêtes (annulation).
func (c *Client) SetContext(ctx context.Context) {
	c.ctxMu.Lock()
	c.ctx = ctx
	c.ctxMu.Unlock()
}

func (c *Client) getContext() context.Context {
	c.ctxMu.Lock()
	ctx := c.ctx
	c.ctxMu.Unlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (c *Client) do(method, path string, body any, params url.Values) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(b)
	}

	if c.baseURL == "" {
		return nil, fmt.Errorf("URL Hydracker non configurée (Réglages)")
	}
	reqURL := c.baseURL + path
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(c.getContext(), method, reqURL, bodyReader)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	// User-Agent descriptif — obligatoire selon l'API Hydracker.
	// Les UA génériques (Go-http-client, curl, python-requests) sont bloqués
	// par le WAF et reçoivent une redirect vers la page de login au lieu
	// de la réponse JSON attendue.
	req.Header.Set("User-Agent", "GoPostTools/2.1 (https://github.com/Gandalfleblanc/Go-Post-Tools)")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	c.waitRateLimit()
	start := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(start)

	// Log entry : on le remplit au fur et à mesure, on le push à la fin.
	entry := APILogEntry{
		Timestamp:  time.Now().Format("15:04:05"),
		Method:     method,
		URL:        reqURL,
		DurationMs: duration.Milliseconds(),
	}
	defer func() {
		if c.OnRequestLog != nil {
			c.OnRequestLog(entry)
		}
	}()

	if err != nil {
		entry.Error = err.Error()
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	entry.Status = resp.StatusCode
	// Preview du body (tronqué) pour debug
	preview := string(data)
	if len(preview) > 500 {
		preview = preview[:500] + "…"
	}
	entry.BodyPreview = preview
	if err != nil {
		entry.Error = err.Error()
		return nil, err
	}

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		return data, nil
	case http.StatusUnauthorized:
		err = fmt.Errorf("unauthorized: vérifiez votre token")
	case http.StatusForbidden:
		err = fmt.Errorf("accès refusé (403)")
	case http.StatusNotFound:
		err = fmt.Errorf("ressource introuvable (404)")
	case http.StatusPaymentRequired:
		err = fmt.Errorf("solde insuffisant (402)")
	case http.StatusUnprocessableEntity:
		err = fmt.Errorf("données invalides (422): %s", string(data))
	default:
		err = fmt.Errorf("erreur HTTP %d: %s", resp.StatusCode, string(data))
	}
	entry.Error = err.Error()
	return nil, err
}

func (c *Client) get(path string, params url.Values) ([]byte, error) {
	return c.do("GET", path, nil, params)
}

func (c *Client) post(path string, body any) ([]byte, error) {
	return c.do("POST", path, body, nil)
}

func (c *Client) put(path string, body any) ([]byte, error) {
	return c.do("PUT", path, body, nil)
}

func (c *Client) delete(path string) ([]byte, error) {
	return c.do("DELETE", path, nil, nil)
}

func intParam(v int) string {
	return strconv.Itoa(v)
}
