package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jeretmccoy/kelma-sync/internal/model"
)

// GetManifest returns lightweight summaries of everything changed on the server
// since the `since` query param. Omit `since` for a full manifest.
func (h *Handler) GetManifest(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	ctx := r.Context()

	// Parse `since`; zero time means "everything".
	var since time.Time
	if s := r.URL.Query().Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "since must be RFC3339")
			return
		}
		since = t
	}

	serverTime := time.Now().UTC()
	m := model.Manifest{ServerTime: serverTime}

	var err error
	if m.Notes, err = manifestNotes(ctx, h.DB, claims.UserID, since); err != nil {
		writeInternalError(w, err)
		return
	}
	if m.Cards, err = manifestCards(ctx, h.DB, claims.UserID, since); err != nil {
		writeInternalError(w, err)
		return
	}
	if m.Notetypes, err = manifestNotetypes(ctx, h.DB, claims.UserID, since); err != nil {
		writeInternalError(w, err)
		return
	}
	if m.Decks, err = manifestDecks(ctx, h.DB, claims.UserID, since); err != nil {
		writeInternalError(w, err)
		return
	}
	if m.Media, err = manifestMedia(ctx, h.DB, claims.UserID, since); err != nil {
		writeInternalError(w, err)
		return
	}
	if m.Tombstones, err = manifestTombstones(ctx, h.DB, claims.UserID, since); err != nil {
		writeInternalError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, m)
}

func manifestNotes(ctx context.Context, db *pgxpool.Pool, userID string, since time.Time) ([]model.ManifestEntry, error) {
	rows, err := db.Query(ctx,
		`SELECT guid, notetype_id, checksum, modified_at, client_modified_at FROM notes
		 WHERE user_id = $1 AND modified_at > $2 ORDER BY modified_at`,
		userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ManifestEntry{}
	for rows.Next() {
		var e model.ManifestEntry
		if err := rows.Scan(&e.GUID, &e.NotetypeID, &e.Checksum, &e.ModifiedAt, &e.ClientModifiedAt); err != nil {
			return nil, err
		}
		utcManifestEntry(&e)
		out = append(out, e)
	}
	return out, rows.Err()
}

func manifestCards(ctx context.Context, db *pgxpool.Pool, userID string, since time.Time) ([]model.ManifestEntry, error) {
	rows, err := db.Query(ctx,
		`SELECT card_id, note_guid, deck_name, ord, modified_at, client_modified_at FROM cards
		 WHERE user_id = $1 AND modified_at > $2 ORDER BY modified_at`,
		userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ManifestEntry{}
	for rows.Next() {
		var e model.ManifestEntry
		var deckName string
		if err := rows.Scan(&e.CardID, &e.NoteGUID, &deckName, &e.Ord, &e.ModifiedAt, &e.ClientModifiedAt); err != nil {
			return nil, err
		}
		e.DeckName = deckName
		e.Checksum = checksum(e.NoteGUID, deckName, e.Ord)
		utcManifestEntry(&e)
		out = append(out, e)
	}
	return out, rows.Err()
}

func manifestNotetypes(ctx context.Context, db *pgxpool.Pool, userID string, since time.Time) ([]model.ManifestEntry, error) {
	rows, err := db.Query(ctx,
		`SELECT notetype_id, name, definition, modified_at, client_modified_at FROM notetypes
		 WHERE user_id = $1 AND modified_at > $2 ORDER BY modified_at`,
		userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ManifestEntry{}
	for rows.Next() {
		var e model.ManifestEntry
		var name string
		var def map[string]any
		if err := rows.Scan(&e.NotetypeID, &name, &def, &e.ModifiedAt, &e.ClientModifiedAt); err != nil {
			return nil, err
		}
		e.Checksum = notetypeChecksum(name, def)
		utcManifestEntry(&e)
		out = append(out, e)
	}
	return out, rows.Err()
}

func manifestDecks(ctx context.Context, db *pgxpool.Pool, userID string, since time.Time) ([]model.ManifestEntry, error) {
	rows, err := db.Query(ctx,
		`SELECT name, config, modified_at, client_modified_at FROM decks
		 WHERE user_id = $1 AND modified_at > $2 ORDER BY modified_at`,
		userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ManifestEntry{}
	for rows.Next() {
		var e model.ManifestEntry
		var cfg map[string]any
		if err := rows.Scan(&e.Name, &cfg, &e.ModifiedAt, &e.ClientModifiedAt); err != nil {
			return nil, err
		}
		e.Checksum = deckChecksum(cfg)
		utcManifestEntry(&e)
		out = append(out, e)
	}
	return out, rows.Err()
}

func manifestMedia(ctx context.Context, db *pgxpool.Pool, userID string, since time.Time) ([]model.ManifestEntry, error) {
	rows, err := db.Query(ctx,
		`SELECT filename, modified_at FROM media
		 WHERE user_id = $1 AND modified_at > $2 ORDER BY modified_at`,
		userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ManifestEntry{}
	for rows.Next() {
		var e model.ManifestEntry
		if err := rows.Scan(&e.Filename, &e.ModifiedAt); err != nil {
			return nil, err
		}
		utcManifestEntry(&e)
		out = append(out, e)
	}
	return out, rows.Err()
}

func manifestTombstones(ctx context.Context, db *pgxpool.Pool, userID string, since time.Time) ([]model.Tombstone, error) {
	rows, err := db.Query(ctx,
		`SELECT id, user_id, type, resource_id, deleted_at, last_client_id FROM tombstones
		 WHERE user_id = $1 AND deleted_at > $2 ORDER BY deleted_at`,
		userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Tombstone{}
	for rows.Next() {
		var t model.Tombstone
		if err := rows.Scan(&t.ID, &t.UserID, &t.Type, &t.ResourceID, &t.DeletedAt, &t.LastClientID); err != nil {
			return nil, err
		}
		t.DeletedAt = t.DeletedAt.UTC()
		out = append(out, t)
	}
	return out, rows.Err()
}

func utcManifestEntry(entry *model.ManifestEntry) {
	entry.ModifiedAt = entry.ModifiedAt.UTC()
	entry.ClientModifiedAt = entry.ClientModifiedAt.UTC()
}
