package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// ImmersionLoginURL is an optional external account authority. Public builds
// do not default to a project-operated service; operators must configure it.
func ImmersionLoginURL() string {
	return strings.TrimSpace(os.Getenv("KELMA_IMMERSION_LOGIN_URL"))
}

// UIDForEmail matches v1 gateway uidForEmail(): sha256(lowercase email)[0:32].
func UIDForEmail(email string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(strings.ToLower(email))))
	return hex.EncodeToString(sum[:])[:32]
}

type VerifiedAccount struct {
	UID    string
	IsPaid bool
}

// VerifyViaImmersion verifies email/password against the configured external
// account service. Returns nil for bad credentials and error for unreachable/5xx.
func VerifyViaImmersion(ctx context.Context, email, password string) (*VerifiedAccount, error) {
	loginURL := ImmersionLoginURL()
	if loginURL == "" {
		return nil, fmt.Errorf("KELMA_IMMERSION_LOGIN_URL is not configured")
	}
	em := strings.TrimSpace(strings.ToLower(email))
	body, _ := json.Marshal(map[string]string{"email": em, "password": password})
	req, err := http.NewRequestWithContext(ctx, "POST", loginURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		var out struct {
			IsPaid bool `json:"isPaid"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return &VerifiedAccount{UID: UIDForEmail(em), IsPaid: out.IsPaid}, nil
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
		return nil, nil
	}
	return nil, fmt.Errorf("immersion returned HTTP %d", resp.StatusCode)
}
