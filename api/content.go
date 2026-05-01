package api

import (
	"encoding/json"
	"fmt"
	"net/url"
)

type TorrentsResult struct {
	Torrents    []TorrentItem `json:"torrents"`
	Count       int           `json:"count"`
	Charged     float64       `json:"charged"`
	AlreadyPaid int           `json:"already_paid"`
}

type NzbsResult struct {
	Nzbs        []Nzb   `json:"nzbs"`
	Count       int     `json:"count"`
	Charged     float64 `json:"charged"`
	AlreadyPaid int     `json:"already_paid"`
}

type LiensResult struct {
	Liens       []Lien  `json:"liens"`
	Count       int     `json:"count"`
	Charged     float64 `json:"charged"`
	AlreadyPaid int     `json:"already_paid"`
}

func contentParams(f ContentFilter) url.Values {
	params := url.Values{}
	if f.Lang > 0 {
		params.Set("lang", intParam(f.Lang))
	}
	if f.Quality > 0 {
		params.Set("qual", intParam(f.Quality))
	}
	if f.Episode > 0 {
		params.Set("episode", intParam(f.Episode))
	}
	if f.Season > 0 {
		params.Set("saison", intParam(f.Season))
	}
	if f.Limit > 0 {
		params.Set("limit", intParam(f.Limit))
	}
	return params
}

// Note : l'API renvoie les réponses à plat (PAS de wrapper "data").
// Exemple : {"torrents":[...],"count":N,"charged":0,"already_paid":0,"status":"success"}

func (c *Client) GetTorrents(titleID int, f ContentFilter) (*TorrentsResult, error) {
	data, err := c.get(fmt.Sprintf("/titles/%d/content/torrents", titleID), contentParams(f))
	if err != nil {
		return nil, err
	}
	LastRawTorrents = string(data)
	var resp TorrentsResult
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

var LastRawTorrents string

// LienDetail : réponse complète de /content/liens/{id}, avec URL débridée +
// statut du débridage (utile pour afficher pourquoi un lien n'a pas d'URL).
type LienDetail struct {
	Lien            Lien   `json:"lien"`
	DirectDL        string `json:"directDL"`
	RawURL          string `json:"raw_url"`
	Debrided        bool   `json:"debrided"`
	DebridError     string `json:"debrid_error"`
	DebridErrorInfo string `json:"debrid_error_detail"`
	LinkSource      string `json:"link_source"`
	Status          string `json:"status"`
}

// GetLienByID appelle /content/liens/{id} et retourne le Lien + l'URL débridée.
//
// Hydracker masque l'URL share dans les listes ET dans le détail brut. Le serveur
// la débride dynamiquement en utilisant la clé 1Fichier configurée sur le compte
// user (Settings Hydracker, pas notre app). Le résultat sort en `directDL` :
//
//	{
//	  "lien": {...},        // métadonnées (id, qualite, taille, etc. — PAS d'URL)
//	  "directDL": "https://a-12.1fichier.com/p2086862583",  // URL débridée directe
//	  "raw_url": null,      // URL share originale (souvent null)
//	  "debrided": true,
//	  "debrid_error": null, // ou message si échec (quota, host non supporté, etc.)
//	  "status": "success"
//	}
//
// On copie directDL (ou raw_url en fallback) dans Lien.URL.
func (c *Client) GetLienByID(id int) (*Lien, error) {
	d, err := c.GetLienDetailByID(id)
	if err != nil {
		return nil, err
	}
	return &d.Lien, nil
}

// GetLienDetailByID retourne la réponse complète avec statut de débridage.
func (c *Client) GetLienDetailByID(id int) (*LienDetail, error) {
	data, err := c.get(fmt.Sprintf("/content/liens/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var resp LienDetail
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	if resp.DirectDL != "" {
		resp.Lien.URL = resp.DirectDL
	} else if resp.RawURL != "" {
		resp.Lien.URL = resp.RawURL
	}
	return &resp, nil
}

// GetTorrentByID appelle /content/torrents/{id} et retourne l'item complet
// avec le download_url réel (les endpoints de liste ne le renvoient pas).
func (c *Client) GetTorrentByID(id int) (*TorrentItem, error) {
	data, err := c.get(fmt.Sprintf("/content/torrents/%d", id), nil)
	if err != nil {
		return nil, err
	}
	// Shape probable : {"torrent": {...}, "charged": 0, ...}
	var resp struct {
		Torrent TorrentItem `json:"torrent"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp.Torrent, nil
}

func (c *Client) GetNzbs(titleID int, f ContentFilter) (*NzbsResult, error) {
	data, err := c.get(fmt.Sprintf("/titles/%d/content/nzbs", titleID), contentParams(f))
	if err != nil {
		return nil, err
	}
	var resp NzbsResult
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetLiens(titleID int, f ContentFilter) (*LiensResult, error) {
	data, err := c.get(fmt.Sprintf("/titles/%d/content/liens", titleID), contentParams(f))
	if err != nil {
		return nil, err
	}
	var resp LiensResult
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GlobalTorrentsResponse — pagination de /torrents (endpoint global, pas /admin).
// Sert pour les stats site-wide : retourne tous les torrents triés par recency.
type GlobalTorrentsResponse struct {
	Pagination struct {
		CurrentPage int           `json:"current_page"`
		LastPage    int           `json:"last_page,omitempty"`
		Total       int           `json:"total,omitempty"`
		Data        []TorrentItem `json:"data"`
	} `json:"pagination"`
}

// GetGlobalTorrents — /torrents (sans /admin) renvoie tous les torrents du site
// (130k+), perPage jusqu'à 100 supporté, tri par created_at desc par défaut.
func (c *Client) GetGlobalTorrents(page, perPage int) (*GlobalTorrentsResponse, error) {
	if perPage <= 0 {
		perPage = 100
	}
	if page <= 0 {
		page = 1
	}
	params := url.Values{}
	params.Set("page", fmt.Sprintf("%d", page))
	params.Set("perPage", fmt.Sprintf("%d", perPage))
	params.Set("orderBy", "created_at")
	params.Set("orderDir", "desc")
	data, err := c.get("/torrents", params)
	if err != nil {
		return nil, err
	}
	var resp GlobalTorrentsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse /torrents: %w", err)
	}
	return &resp, nil
}

// GetNfo récupère le NFO HTML d'un item via /{kind}/{id} qui retourne
// {model: {nfo: "..."}}. kind = "torrents" | "liens" | "nzbs".
func (c *Client) GetNfo(kind string, id int) (string, error) {
	data, err := c.get(fmt.Sprintf("/%s/%d", kind, id), nil)
	if err != nil {
		return "", err
	}
	var resp struct {
		Model struct {
			Nfo string `json:"nfo"`
		} `json:"model"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	return resp.Model.Nfo, nil
}
