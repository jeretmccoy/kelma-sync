package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jeretmccoy/kelma-sync/internal/model"
)

// GetDeck returns a deck's config.
func (h *Handler) GetDeck(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	name := r.PathValue("name")
	d, err := h.loadDeck(r, claims.UserID, name)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "deck not found")
		return
	}
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

type putDeckRequest struct {
	Config           map[string]any `json:"config"`
	ClientModifiedAt time.Time      `json:"client_modified_at"`
	BaseChecksum     string         `json:"base_checksum"`
}

// PutDeck upserts a deck with the same conflict semantics as notes.
func (h *Handler) PutDeck(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	name := r.PathValue("name")
	var req putDeckRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Config == nil {
		req.Config = map[string]any{}
	}
	newChecksum := deckChecksum(req.Config)

	existing, err := h.loadDeck(r, claims.UserID, name)
	notFound := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !notFound {
		writeInternalError(w, err)
		return
	}
	if !notFound && !forceOverride(r) && existing.Checksum != req.BaseChecksum {
		if existing.Checksum != newChecksum {
			writeJSON(w, http.StatusConflict, model.ConflictResponse{
				Error:  "conflict",
				Server: existing,
				Client: req,
			})
			return
		}
	}

	saved, err := h.upsertDeck(r.Context(), claims.UserID, claims.ClientID, name, req, newChecksum, time.Now().UTC())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// DeleteDeck soft-deletes a deck.
func (h *Handler) DeleteDeck(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	name := r.PathValue("name")
	_, err := h.DB.Exec(r.Context(),
		`DELETE FROM decks WHERE user_id = $1 AND name = $2`, claims.UserID, name)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if err := h.writeTombstone(r, claims.UserID, "deck", name, claims.ClientID); err != nil {
		writeInternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) loadDeck(r *http.Request, userID, name string) (model.Deck, error) {
	var d model.Deck
	err := h.DB.QueryRow(r.Context(),
		`SELECT d.id, d.user_id, d.name, d.config, d.checksum,
		        d.modified_at, d.client_modified_at,
		        COALESCE(d.last_client_id::text, ''), COALESCE(c.label, '')
		 FROM decks d LEFT JOIN clients c ON c.id = d.last_client_id
		 WHERE d.user_id = $1 AND d.name = $2`,
		userID, name,
	).Scan(&d.ID, &d.UserID, &d.Name, &d.Config, &d.Checksum,
		&d.ModifiedAt, &d.ClientModifiedAt, &d.LastClientID, &d.LastClientLabel)
	return d, err
}
