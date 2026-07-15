package handler

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jeretmccoy/kelma-sync/internal/storage"
)

const maxMediaBytes = 100 << 20 // 100 MiB

// storageKey derives the R2 object key for a user's media file.
func storageKey(userID, filename string) string {
	return userID + "/" + filename
}

// HeadMedia reports whether the server has a media file.
func (h *Handler) HeadMedia(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	filename := r.PathValue("filename")
	var exists bool
	err := h.DB.QueryRow(r.Context(),
		`SELECT true FROM media WHERE user_id = $1 AND filename = $2 AND storage_key IS NOT NULL`,
		claims.UserID, filename,
	).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// GetMedia streams a media file's bytes from R2.
func (h *Handler) GetMedia(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	filename := r.PathValue("filename")

	var key *string
	err := h.DB.QueryRow(r.Context(),
		`SELECT storage_key FROM media WHERE user_id = $1 AND filename = $2`,
		claims.UserID, filename,
	).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) || key == nil {
		writeError(w, http.StatusNotFound, "not_found", "media not found")
		return
	}
	if err != nil {
		writeInternalError(w, err)
		return
	}

	body, contentType, err := h.Storage.Get(r.Context(), *key)
	if errors.Is(err, storage.ErrNotFound) {
		// The DB row exists but the blob is gone (e.g. dev restart wiped an
		// in-memory store). Return 404 so the client treats the file as absent
		// and re-uploads its local copy, instead of failing the whole sync.
		writeError(w, http.StatusNotFound, "not_found", "media blob missing")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "storage read failed")
		return
	}
	defer body.Close()

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

// PutMedia uploads a media file to R2 and records its metadata. Idempotent.
func (h *Handler) PutMedia(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	filename := r.PathValue("filename")

	r.Body = http.MaxBytesReader(w, r.Body, maxMediaBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "media exceeds size limit")
		return
	}

	key := storageKey(claims.UserID, filename)
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if err := h.Storage.Put(r.Context(), key, byteReader(data), int64(len(data)), contentType); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "storage write failed")
		return
	}

	now := time.Now().UTC()
	_, err = h.DB.Exec(r.Context(),
		`INSERT INTO media (user_id, filename, size_bytes, storage_key, modified_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (user_id, filename) DO UPDATE SET
		     size_bytes  = EXCLUDED.size_bytes,
		     storage_key = EXCLUDED.storage_key,
		     modified_at = EXCLUDED.modified_at`,
		claims.UserID, filename, len(data), key, now)
	if err != nil {
		writeInternalError(w, err)
		return
	}

	_, _ = h.DB.Exec(r.Context(),
		`DELETE FROM tombstones WHERE user_id = $1 AND type = 'media' AND resource_id = $2`,
		claims.UserID, filename)

	writeJSON(w, http.StatusCreated, map[string]any{
		"filename":   filename,
		"size_bytes": len(data),
	})
}

// DeleteMedia removes a media file and writes a tombstone.
func (h *Handler) DeleteMedia(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	filename := r.PathValue("filename")

	var key *string
	err := h.DB.QueryRow(r.Context(),
		`DELETE FROM media WHERE user_id = $1 AND filename = $2 RETURNING storage_key`,
		claims.UserID, filename,
	).Scan(&key)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeInternalError(w, err)
		return
	}
	if key != nil {
		_ = h.Storage.Delete(r.Context(), *key)
	}
	if err := h.writeTombstone(r, claims.UserID, "media", filename, claims.ClientID); err != nil {
		writeInternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
