package handler

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	"github.com/jeretmccoy/kelma-sync/internal/auth"
)

// Internal endpoints let the trusted Kelma Immersion backend mint sync tokens
// and read storage usage for its users without ever handling their passwords.
// They mirror the v1 gateway's /internal/hostkey and /internal/usage contract:
//   POST body {email, isPaid} + x-internal-secret header.
// Disabled entirely unless KELMA_INTERNAL_SECRET is set.

func internalSecret() string {
	return strings.TrimSpace(os.Getenv("KELMA_INTERNAL_SECRET"))
}

// requireInternal guards a handler with the shared-secret header.
func requireInternal(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secret := internalSecret()
		if secret == "" {
			writeError(w, http.StatusNotFound, "not_found", "internal API is not enabled")
			return
		}
		provided := r.Header.Get("x-internal-secret")
		if len(provided) != len(secret) || subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized", "bad internal secret")
			return
		}
		next(w, r)
	}
}

type internalTokenRequest struct {
	Email  string `json:"email"`
	IsPaid bool   `json:"isPaid"`
}

type internalTokenResponse struct {
	// Named hkey to match the v1 gateway response the backend already parses.
	HKey   string `json:"hkey"`
	UserID string `json:"user_id"`
}

// InternalToken mints a bearer token for the given account email. The caller
// (Immersion backend) has already authenticated the user; their uid is derived
// the same way as interactive v2 login (sha256(email)[:32]).
func (h *Handler) InternalToken(w http.ResponseWriter, r *http.Request) {
	var req internalTokenRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "email required")
		return
	}

	userID := auth.UIDForEmail(email)
	if _, err := h.DB.Exec(r.Context(),
		`INSERT INTO users (id, username, password_hash) VALUES ($1, $2, NULL)
		 ON CONFLICT (id) DO UPDATE SET username = EXCLUDED.username`,
		userID, email,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "user upsert failed")
		return
	}

	var clientID string
	if err := h.DB.QueryRow(r.Context(),
		`INSERT INTO clients (user_id, label, last_seen)
		 VALUES ($1, 'kelma-immersion-web', now())
		 ON CONFLICT (user_id, label) DO UPDATE SET last_seen = now()
		 RETURNING id`,
		userID,
	).Scan(&clientID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "client upsert failed")
		return
	}

	raw, hashed, err := auth.GenerateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "token generation failed")
		return
	}
	if _, err := h.DB.Exec(r.Context(),
		`INSERT INTO tokens (user_id, client_id, token_hash, expires_at)
		 VALUES ($1, $2, $3, now() + interval '1 year')`,
		userID, clientID, hashed,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "token store failed")
		return
	}

	writeJSON(w, http.StatusOK, internalTokenResponse{HKey: raw, UserID: userID})
}

type internalUsageResponse struct {
	UsedBytes  int64 `json:"usedBytes"`
	QuotaBytes int64 `json:"quotaBytes"`
}

const (
	freeQuotaBytes = int64(5) << 30  // 5 GiB
	paidQuotaBytes = int64(20) << 30 // 20 GiB
)

// InternalUsage reports a user's stored bytes and quota for the web UI meter.
// Usage = media blob bytes + note/card/notetype/deck content bytes.
func (h *Handler) InternalUsage(w http.ResponseWriter, r *http.Request) {
	var req internalTokenRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "email required")
		return
	}
	userID := auth.UIDForEmail(email)

	var used int64
	if err := h.DB.QueryRow(r.Context(), `
		SELECT COALESCE((SELECT SUM(size_bytes) FROM media WHERE user_id = $1), 0)
		     + COALESCE((SELECT SUM(octet_length(fields::text) + octet_length(tags::text)) FROM notes WHERE user_id = $1), 0)
		     + COALESCE((SELECT SUM(octet_length(scheduling::text)) FROM cards WHERE user_id = $1), 0)
		     + COALESCE((SELECT SUM(octet_length(definition::text)) FROM notetypes WHERE user_id = $1), 0)
		     + COALESCE((SELECT SUM(octet_length(config::text)) FROM decks WHERE user_id = $1), 0)
	`, userID).Scan(&used); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "usage query failed")
		return
	}

	quota := freeQuotaBytes
	if req.IsPaid {
		quota = paidQuotaBytes
	}
	writeJSON(w, http.StatusOK, internalUsageResponse{UsedBytes: used, QuotaBytes: quota})
}
