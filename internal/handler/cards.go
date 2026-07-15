package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jeretmccoy/kelma-sync/internal/model"
)

// GetCard returns the full card record for a card_id.
func (h *Handler) GetCard(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	cardID, err := strconv.ParseInt(r.PathValue("card_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "card_id must be an integer")
		return
	}
	c, err := h.loadCard(r, claims.UserID, cardID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "card not found")
		return
	}
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

type putCardRequest struct {
	NoteGUID         string         `json:"note_guid"`
	DeckName         string         `json:"deck_name"`
	Ord              int            `json:"ord"`
	Scheduling       map[string]any `json:"scheduling"`
	ClientModifiedAt time.Time      `json:"client_modified_at"`
}

// PutCard upserts a card using per-card timestamp comparison. If the stored
// card was modified more recently by the client clock, the incoming push is
// ignored and the existing record is returned (silent newest-wins).
func (h *Handler) PutCard(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	cardID, err := strconv.ParseInt(r.PathValue("card_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "card_id must be an integer")
		return
	}
	var req putCardRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Scheduling == nil {
		req.Scheduling = map[string]any{}
	}
	now, normalizedClientTime := utcWriteTimes(time.Now(), req.ClientModifiedAt)
	req.ClientModifiedAt = normalizedClientTime

	// Resolve by logical identity so a device pushing its own card_id updates the
	// existing logical card instead of creating a duplicate.
	existing, err := h.loadCardLogical(r, claims.UserID, req.NoteGUID, req.Ord)
	notFound := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !notFound {
		writeInternalError(w, err)
		return
	}
	// Newest-wins: if the stored card is newer, keep it.
	if !notFound && existing.ClientModifiedAt.After(req.ClientModifiedAt) {
		writeJSON(w, http.StatusOK, existing)
		return
	}
	if !notFound {
		cardID = existing.CardID // keep canonical id
	}

	saved, err := h.upsertCard(r.Context(), claims.UserID, claims.ClientID, cardID, req, now)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// DeleteCard soft-deletes a card by writing a tombstone.
func (h *Handler) DeleteCard(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	cardIDStr := r.PathValue("card_id")
	cardID, err := strconv.ParseInt(cardIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "card_id must be an integer")
		return
	}
	_, err = h.DB.Exec(r.Context(),
		`DELETE FROM cards WHERE user_id = $1 AND card_id = $2`, claims.UserID, cardID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if err := h.writeTombstone(r, claims.UserID, "card", cardIDStr, claims.ClientID); err != nil {
		writeInternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) loadCard(r *http.Request, userID string, cardID int64) (model.Card, error) {
	var c model.Card
	err := h.DB.QueryRow(r.Context(),
		`SELECT c.id, c.user_id, c.card_id, c.note_guid, c.deck_name, c.ord, c.scheduling,
		        c.modified_at, c.client_modified_at,
		        COALESCE(c.last_client_id::text, ''), COALESCE(cl.label, '')
		 FROM cards c LEFT JOIN clients cl ON cl.id = c.last_client_id
		 WHERE c.user_id = $1 AND c.card_id = $2`,
		userID, cardID,
	).Scan(&c.ID, &c.UserID, &c.CardID, &c.NoteGUID, &c.DeckName, &c.Ord, &c.Scheduling,
		&c.ModifiedAt, &c.ClientModifiedAt, &c.LastClientID, &c.LastClientLabel)
	return c, err
}

// loadCardLogical loads a card by its cross-device identity (note_guid, ord).
func (h *Handler) loadCardLogical(r *http.Request, userID, noteGUID string, ord int) (model.Card, error) {
	var c model.Card
	err := h.DB.QueryRow(r.Context(),
		`SELECT c.id, c.user_id, c.card_id, c.note_guid, c.deck_name, c.ord, c.scheduling,
		        c.modified_at, c.client_modified_at,
		        COALESCE(c.last_client_id::text, ''), COALESCE(cl.label, '')
		 FROM cards c LEFT JOIN clients cl ON cl.id = c.last_client_id
		 WHERE c.user_id = $1 AND c.note_guid = $2 AND c.ord = $3`,
		userID, noteGUID, ord,
	).Scan(&c.ID, &c.UserID, &c.CardID, &c.NoteGUID, &c.DeckName, &c.Ord, &c.Scheduling,
		&c.ModifiedAt, &c.ClientModifiedAt, &c.LastClientID, &c.LastClientLabel)
	return c, err
}
