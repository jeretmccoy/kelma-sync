package handler

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jeretmccoy/kelma-sync/internal/auth"
)

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type registerResponse struct {
	UserID string `json:"user_id"`
}

// localAuthEnabled defaults to local accounts for a self-contained open-source
// deployment. Operators can select an external account authority explicitly.
func localAuthEnabled() bool {
	mode := strings.TrimSpace(strings.ToLower(os.Getenv("KELMA_AUTH_MODE")))
	return mode == "" || mode == "local"
}

// Register creates a local user account. Registration is disabled when an
// external account authority is selected.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if !localAuthEnabled() {
		writeError(w, http.StatusForbidden, "registration_disabled",
			"registration is handled by the configured account service")
		return
	}
	var req registerRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "username and password required")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "hash failed")
		return
	}

	userID := auth.UIDForEmail(req.Username)
	err = h.DB.QueryRow(r.Context(),
		`INSERT INTO users (id, username, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		userID, strings.ToLower(req.Username), hash,
	).Scan(&userID)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "username_taken", "username already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "insert failed")
		return
	}

	writeJSON(w, http.StatusCreated, registerResponse{UserID: userID})
}

type loginRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	ClientLabel string `json:"client_label"`
}

type loginResponse struct {
	Token    string `json:"token"`
	ClientID string `json:"client_id"`
}

// Login verifies credentials, upserts the client, and issues a token.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.ClientLabel = strings.TrimSpace(req.ClientLabel)
	if req.Username == "" || req.Password == "" || req.ClientLabel == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "username, password, client_label required")
		return
	}

	// Local auth is self-contained. An external account authority can be selected
	// explicitly for an ecosystem deployment that already has user accounts.
	var userID string
	var err error
	if localAuthEnabled() {
		var passwordHash string
		err := h.DB.QueryRow(r.Context(),
			`SELECT id, COALESCE(password_hash, '') FROM users WHERE username = $1`, strings.ToLower(req.Username),
		).Scan(&userID, &passwordHash)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "lookup failed")
			return
		}
		if err := auth.CheckPassword(req.Password, passwordHash); err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
			return
		}
	} else {
		acct, err := auth.VerifyViaImmersion(r.Context(), req.Username, req.Password)
		if err != nil {
			log.Printf("external account authority error: %v", err)
			writeError(w, http.StatusBadGateway, "account_service_unavailable", "account service unavailable")
			return
		}
		if acct == nil {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
			return
		}
		userID = acct.UID
		// Ensure a local user row exists for foreign keys. Password stays NULL;
		// external credentials remain with the configured account authority.
		_, err = h.DB.Exec(r.Context(),
			`INSERT INTO users (id, username, password_hash) VALUES ($1, $2, NULL)
			 ON CONFLICT (id) DO UPDATE SET username = EXCLUDED.username`,
			userID, strings.ToLower(req.Username),
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "user upsert failed")
			return
		}
	}

	// Upsert the client by (user_id, label), refreshing last_seen.
	var clientID string
	err = h.DB.QueryRow(r.Context(),
		`INSERT INTO clients (user_id, label, last_seen)
		 VALUES ($1, $2, now())
		 ON CONFLICT (user_id, label)
		 DO UPDATE SET last_seen = now()
		 RETURNING id`,
		userID, req.ClientLabel,
	).Scan(&clientID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "client upsert failed")
		return
	}

	// Issue a token.
	raw, hashed, err := auth.GenerateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "token generation failed")
		return
	}
	_, err = h.DB.Exec(r.Context(),
		`INSERT INTO tokens (user_id, client_id, token_hash, expires_at)
		 VALUES ($1, $2, $3, now() + interval '1 year')`,
		userID, clientID, hashed,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "token store failed")
		return
	}

	writeJSON(w, http.StatusOK, loginResponse{Token: raw, ClientID: clientID})
}

// Logout revokes the token used to make the request.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	header := r.Header.Get("Authorization")
	raw := strings.TrimPrefix(header, "Bearer ")
	hashed := auth.HashToken(raw)
	_, err := h.DB.Exec(r.Context(), `DELETE FROM tokens WHERE token_hash = $1`, hashed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "logout failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
