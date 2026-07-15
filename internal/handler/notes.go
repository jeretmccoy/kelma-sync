package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jeretmccoy/kelma-sync/internal/model"
)

// GetNote returns the full note record for a guid.
func (h *Handler) GetNote(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	guid := r.PathValue("guid")

	n, err := h.loadNote(r, claims.UserID, guid)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "note not found")
		return
	}
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

type putNoteRequest struct {
	NotetypeID       int64     `json:"notetype_id"`
	Fields           []string  `json:"fields"`
	Tags             []string  `json:"tags"`
	ClientModifiedAt time.Time `json:"client_modified_at"`
	// BaseChecksum is the checksum of the version the client last synced. The
	// server accepts the write only if the stored note still has this checksum
	// (optimistic concurrency). Empty means the client believes the note is new.
	BaseChecksum string `json:"base_checksum"`
}

// PutNote creates or updates a note. Returns 409 on conflict unless
// Force-Override is set.
func (h *Handler) PutNote(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	guid := r.PathValue("guid")

	var req putNoteRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}
	newChecksum := checksum(req.Fields, req.Tags)

	// Load the existing note (if any) to detect conflicts.
	existing, err := h.loadNote(r, claims.UserID, guid)
	notFound := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !notFound {
		writeInternalError(w, err)
		return
	}

	// Conflict: the note exists on the server with a different checksum than the
	// client's base (what it last synced). That means another client wrote it in
	// the meantime — the two sides diverged. Force-Override bypasses the check.
	if !notFound && !forceOverride(r) && existing.Checksum != req.BaseChecksum {
		// If the server already holds exactly what the client is pushing, it's a
		// harmless re-push, not a conflict.
		if existing.Checksum != newChecksum {
			writeJSON(w, http.StatusConflict, model.ConflictResponse{
				Error:  "conflict",
				Server: existing,
				Client: req,
			})
			return
		}
	}

	saved, err := h.upsertNote(r.Context(), claims.UserID, claims.ClientID, guid, req, newChecksum, time.Now().UTC())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// DeleteNote soft-deletes a note by writing a tombstone.
func (h *Handler) DeleteNote(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	guid := r.PathValue("guid")

	_, err := h.DB.Exec(r.Context(),
		`DELETE FROM notes WHERE user_id = $1 AND guid = $2`, claims.UserID, guid)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if err := h.writeTombstone(r, claims.UserID, "note", guid, claims.ClientID); err != nil {
		writeInternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// loadNote fetches a note including its client label.
func (h *Handler) loadNote(r *http.Request, userID, guid string) (model.Note, error) {
	var n model.Note
	err := h.DB.QueryRow(r.Context(),
		`SELECT n.id, n.user_id, n.guid, n.notetype_id, n.fields, n.tags, n.checksum,
		        n.modified_at, n.client_modified_at,
		        COALESCE(n.last_client_id::text, ''), COALESCE(c.label, '')
		 FROM notes n LEFT JOIN clients c ON c.id = n.last_client_id
		 WHERE n.user_id = $1 AND n.guid = $2`,
		userID, guid,
	).Scan(&n.ID, &n.UserID, &n.GUID, &n.NotetypeID, &n.Fields, &n.Tags, &n.Checksum,
		&n.ModifiedAt, &n.ClientModifiedAt, &n.LastClientID, &n.LastClientLabel)
	return n, err
}
