package model

import "time"

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type Client struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Label     string     `json:"label"`
	LastSeen  *time.Time `json:"last_seen"`
	CreatedAt time.Time  `json:"created_at"`
}

type Token struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	ClientID  string     `json:"client_id"`
	TokenHash string     `json:"-"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
}

type Note struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	GUID             string    `json:"guid"`
	NotetypeID       int64     `json:"notetype_id"`
	Fields           []string  `json:"fields"`
	Tags             []string  `json:"tags"`
	Checksum         string    `json:"checksum"`
	ModifiedAt       time.Time `json:"modified_at"`
	ClientModifiedAt time.Time `json:"client_modified_at"`
	LastClientID     string    `json:"last_client_id"`
	LastClientLabel  string    `json:"last_client_label"`
}

type Card struct {
	ID               string         `json:"id"`
	UserID           string         `json:"user_id"`
	CardID           int64          `json:"card_id"`
	NoteGUID         string         `json:"note_guid"`
	DeckName         string         `json:"deck_name"`
	Ord              int            `json:"ord"`
	Scheduling       map[string]any `json:"scheduling"`
	ModifiedAt       time.Time      `json:"modified_at"`
	ClientModifiedAt time.Time      `json:"client_modified_at"`
	LastClientID     string         `json:"last_client_id"`
	LastClientLabel  string         `json:"last_client_label"`
}

type Notetype struct {
	ID               string         `json:"id"`
	UserID           string         `json:"user_id"`
	NotetypeID       int64          `json:"notetype_id"`
	Name             string         `json:"name"`
	Definition       map[string]any `json:"definition"`
	Checksum         string         `json:"checksum"`
	ModifiedAt       time.Time      `json:"modified_at"`
	ClientModifiedAt time.Time      `json:"client_modified_at"`
	LastClientID     string         `json:"last_client_id"`
	LastClientLabel  string         `json:"last_client_label"`
}

type Deck struct {
	ID               string         `json:"id"`
	UserID           string         `json:"user_id"`
	Name             string         `json:"name"`
	Config           map[string]any `json:"config"`
	Checksum         string         `json:"checksum"`
	ModifiedAt       time.Time      `json:"modified_at"`
	ClientModifiedAt time.Time      `json:"client_modified_at"`
	LastClientID     string         `json:"last_client_id"`
	LastClientLabel  string         `json:"last_client_label"`
}

type Media struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Filename   string    `json:"filename"`
	SizeBytes  int64     `json:"size_bytes"`
	StorageKey *string   `json:"storage_key"`
	RefCount   int       `json:"ref_count"`
	ModifiedAt time.Time `json:"modified_at"`
}

type Tombstone struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	Type         string    `json:"type"`
	ResourceID   string    `json:"resource_id"`
	DeletedAt    time.Time `json:"deleted_at"`
	LastClientID *string   `json:"last_client_id"`
}

// ManifestEntry is the lightweight summary returned by GET /v2/sync/manifest.
type ManifestEntry struct {
	GUID       string    `json:"guid,omitempty"`
	CardID     int64     `json:"card_id,omitempty"`
	NotetypeID int64     `json:"notetype_id,omitempty"`
	Name       string    `json:"name,omitempty"`
	Filename   string    `json:"filename,omitempty"`
	Checksum   string    `json:"checksum,omitempty"`
	ModifiedAt time.Time `json:"modified_at"`
	// Source timestamp lets two-source clients resolve a strictly newer copy.
	// Card logical identity is also exposed so clients can compare scheduling
	// without a batch pull.
	NoteGUID         string    `json:"note_guid,omitempty"`
	DeckName         string    `json:"deck_name,omitempty"`
	Ord              int       `json:"ord"`
	ClientModifiedAt time.Time `json:"client_modified_at,omitempty"`
}

type Manifest struct {
	Notes      []ManifestEntry `json:"notes"`
	Cards      []ManifestEntry `json:"cards"`
	Notetypes  []ManifestEntry `json:"notetypes"`
	Decks      []ManifestEntry `json:"decks"`
	Media      []ManifestEntry `json:"media"`
	Tombstones []Tombstone     `json:"tombstones"`
	ServerTime time.Time       `json:"server_time"`
}

// ConflictResponse is returned as 409 when a push conflicts.
type ConflictResponse struct {
	Error  string `json:"error"`
	Server any    `json:"server"`
	Client any    `json:"client"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}
