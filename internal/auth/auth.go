package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrInvalidToken = errors.New("invalid or expired token")

// HashPassword bcrypt-hashes a plaintext password.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword compares a plaintext password against a bcrypt hash.
func CheckPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// GenerateToken creates a cryptographically random token and returns both the
// raw token (to send to the client) and its SHA-256 hash (to store in the DB).
func GenerateToken() (raw, hashed string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("rand.Read: %w", err)
	}
	raw = hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(raw))
	hashed = hex.EncodeToString(sum[:])
	return raw, hashed, nil
}

// HashToken returns the SHA-256 hex hash of a raw token string.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Claims holds the authenticated identity extracted from a valid token.
type Claims struct {
	UserID   string
	ClientID string
}

// Authenticate extracts the Bearer token from the request, looks it up in the
// database, and returns the associated Claims. Returns ErrInvalidToken if the
// token is missing, unknown, or expired.
func Authenticate(ctx context.Context, r *http.Request, pool *pgxpool.Pool) (*Claims, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return nil, ErrInvalidToken
	}
	raw := strings.TrimPrefix(header, "Bearer ")
	hashed := HashToken(raw)

	var userID, clientID string
	var expiresAt *time.Time
	err := pool.QueryRow(ctx,
		`SELECT user_id, client_id, expires_at FROM tokens WHERE token_hash = $1`,
		hashed,
	).Scan(&userID, &clientID, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidToken
	}
	if err != nil {
		return nil, fmt.Errorf("token lookup: %w", err)
	}
	if expiresAt != nil && time.Now().After(*expiresAt) {
		return nil, ErrInvalidToken
	}
	return &Claims{UserID: userID, ClientID: clientID}, nil
}
