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

// GetLienByID appelle /content/liens/{id} et retourne le Lien complet avec
// l'URL de partage réelle (les endpoints de liste ne la renvoient pas).
func (c *Client) GetLienByID(id int) (*Lien, error) {
	data, err := c.get(fmt.Sprintf("/content/liens/%d", id), nil)
	if err != nil {
		return nil, err
	}
	// Shape probable : {"lien": {...}, "charged": 0, ...}
	var resp struct {
		Lien Lien `json:"lien"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp.Lien, nil
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
