// Package api : endpoints admin (/admin/liens, /admin/torrents) + mutations.
// Les endpoints /admin/* requièrent un token admin côté Hydracker, un user
// standard reçoit 403 ou du HTML (notre parser JSON échouera proprement).
package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// AdminLiensResponse est la page paginée retournée par /admin/liens.
type AdminLiensResponse struct {
	Pagination struct {
		CurrentPage int    `json:"current_page"`
		LastPage    int    `json:"last_page,omitempty"`
		Total       int    `json:"total,omitempty"`
		Data        []Lien `json:"data"`
	} `json:"pagination"`
}

// AdminTorrentsResponse est la page paginée retournée par /admin/torrents.
type AdminTorrentsResponse struct {
	Pagination struct {
		CurrentPage int           `json:"current_page"`
		LastPage    int           `json:"last_page,omitempty"`
		Total       int           `json:"total,omitempty"`
		Data        []TorrentItem `json:"data"`
	} `json:"pagination"`
}

// ListAdminLiens — /admin/liens paginé. idUser filtre optionnel par pseudo.
func (c *Client) ListAdminLiens(idUser string, page int) (*AdminLiensResponse, error) {
	params := url.Values{}
	if idUser != "" {
		params.Set("id_user", idUser)
	}
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
	}
	params.Set("perPage", "50")
	data, err := c.get("/admin/liens", params)
	if err != nil {
		return nil, err
	}
	var resp AdminLiensResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse /admin/liens: %w", err)
	}
	return &resp, nil
}

// ListAdminTorrents — /admin/torrents paginé. uploader filtre par pseudo.
func (c *Client) ListAdminTorrents(uploader string, page int) (*AdminTorrentsResponse, error) {
	params := url.Values{}
	if uploader != "" {
		params.Set("author", uploader)
	}
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
	}
	params.Set("perPage", "50")
	data, err := c.get("/admin/torrents", params)
	if err != nil {
		return nil, err
	}
	var resp AdminTorrentsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse /admin/torrents: %w", err)
	}
	return &resp, nil
}

// --- Mutations (DELETE + PUT) ---
// On vérifie explicitement la réponse : le serveur renvoie parfois 200+success
// pour des IDs inexistants (comportement idempotent-ish à surveiller).

// DeleteLien DELETE /liens/{id}. Vérifie status:success dans la réponse.
func (c *Client) DeleteLien(id int) error {
	data, err := c.delete(fmt.Sprintf("/liens/%d", id))
	if err != nil {
		return err
	}
	return checkMutationResponse(data, "delete lien")
}

// DeleteTorrent DELETE /torrents/{id}.
func (c *Client) DeleteTorrent(id int) error {
	data, err := c.delete(fmt.Sprintf("/torrents/%d", id))
	if err != nil {
		return err
	}
	return checkMutationResponse(data, "delete torrent")
}

// DeleteNzb DELETE /nzb/{id}.
func (c *Client) DeleteNzb(id int) error {
	data, err := c.delete(fmt.Sprintf("/nzb/%d", id))
	if err != nil {
		return err
	}
	return checkMutationResponse(data, "delete nzb")
}

// UpdateLienPayload — champs modifiables d'un lien.
// Valeurs 0 / nil ignorées côté serveur (omitempty).
type UpdateLienPayload struct {
	Quality int  `json:"qualite,omitempty"`
	Lang    int  `json:"lang_id,omitempty"`
	Season  int  `json:"saison,omitempty"`
	Episode int  `json:"episode,omitempty"`
	Active  *int `json:"active,omitempty"` // 0/1 pour désactiver/activer, nil pour no-op
}

// UpdateLien PUT /liens/{id}.
func (c *Client) UpdateLien(id int, p UpdateLienPayload) error {
	data, err := c.put(fmt.Sprintf("/liens/%d", id), p)
	if err != nil {
		return err
	}
	return checkMutationResponse(data, "update lien")
}

// UpdateTorrentPayload — champs modifiables d'un torrent.
type UpdateTorrentPayload struct {
	Quality int  `json:"qualite,omitempty"`
	Lang    int  `json:"lang_id,omitempty"`
	Season  int  `json:"saison,omitempty"`
	Episode int  `json:"episode,omitempty"`
	Active  *int `json:"active,omitempty"`
}

// UpdateTorrent PUT /torrents/{id}.
func (c *Client) UpdateTorrent(id int, p UpdateTorrentPayload) error {
	data, err := c.put(fmt.Sprintf("/torrents/%d", id), p)
	if err != nil {
		return err
	}
	return checkMutationResponse(data, "update torrent")
}

// checkMutationResponse extrait status/message et renvoie une erreur claire
// si le status n'est pas "success".
func checkMutationResponse(data []byte, action string) error {
	var resp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		// Si la réponse n'est pas un JSON, on considère que c'est un échec avec le
		// raw data tronqué pour déboguer.
		raw := string(data)
		if len(raw) > 200 {
			raw = raw[:200] + "…"
		}
		return fmt.Errorf("%s: réponse invalide (%s)", action, raw)
	}
	if resp.Status != "" && resp.Status != "success" {
		return fmt.Errorf("%s: %s", action, resp.Message)
	}
	// Status "success" OU status vide (certaines routes renvoient juste l'objet mis à jour)
	return nil
}
