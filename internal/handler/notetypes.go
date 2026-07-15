package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jeretmccoy/kelma-sync/internal/model"
)

// GetNotetype returns a notetype definition.
func (h *Handler) GetNotetype(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	id, err := strconv.ParseInt(r.PathValue("notetype_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "notetype_id must be an integer")
		return
	}
	nt, err := h.loadNotetype(r, claims.UserID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "notetype not found")
		return
	}
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nt)
}

type putNotetypeRequest struct {
	Name             string         `json:"name"`
	Definition       map[string]any `json:"definition"`
	ClientModifiedAt time.Time      `json:"client_modified_at"`
	BaseChecksum     string         `json:"base_checksum"`
}

// PutNotetype upserts a notetype with the same conflict semantics as notes.
func (h *Handler) PutNotetype(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	id, err := strconv.ParseInt(r.PathValue("notetype_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "notetype_id must be an integer")
		return
	}
	var req putNotetypeRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	newChecksum := notetypeChecksum(req.Name, req.Definition)

	existing, err := h.loadNotetype(r, claims.UserID, id)
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

	saved, err := h.upsertNotetype(r.Context(), claims.UserID, claims.ClientID, id, req, newChecksum, time.Now().UTC())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// DeleteNotetype soft-deletes a notetype.
func (h *Handler) DeleteNotetype(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	idStr := r.PathValue("notetype_id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "notetype_id must be an integer")
		return
	}
	_, err = h.DB.Exec(r.Context(),
		`DELETE FROM notetypes WHERE user_id = $1 AND notetype_id = $2`, claims.UserID, id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if err := h.writeTombstone(r, claims.UserID, "notetype", idStr, claims.ClientID); err != nil {
		writeInternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) loadNotetype(r *http.Request, userID string, id int64) (model.Notetype, error) {
	var nt model.Notetype
	err := h.DB.QueryRow(r.Context(),
		`SELECT n.id, n.user_id, n.notetype_id, n.name, n.definition, n.checksum,
		        n.modified_at, n.client_modified_at,
		        COALESCE(n.last_client_id::text, ''), COALESCE(c.label, '')
		 FROM notetypes n LEFT JOIN clients c ON c.id = n.last_client_id
		 WHERE n.user_id = $1 AND n.notetype_id = $2`,
		userID, id,
	).Scan(&nt.ID, &nt.UserID, &nt.NotetypeID, &nt.Name, &nt.Definition, &nt.Checksum,
		&nt.ModifiedAt, &nt.ClientModifiedAt, &nt.LastClientID, &nt.LastClientLabel)
	return nt, err
}
