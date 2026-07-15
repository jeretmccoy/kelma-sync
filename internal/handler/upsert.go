package handler

import (
	"context"
	"time"

	"github.com/jeretmccoy/kelma-sync/internal/model"
)

// This file holds the single source of truth for writing each resource type.
// Both the single-item PUT handlers and the batch endpoint call these, so the
// upsert SQL lives in exactly one place per resource.
//
// Each helper performs the INSERT ... ON CONFLICT DO UPDATE, clears any
// tombstone for the resource, and returns the saved record. Conflict detection
// is the caller's responsibility (it happens before the write).

const maxFutureClockSkew = 5 * time.Minute

// utcWriteTimes makes every persisted timestamp explicitly UTC. The server's
// authenticated clock is authoritative: a client clock far in the future is
// clamped so it cannot poison newest-wins reconciliation indefinitely.
func utcWriteTimes(now, clientModifiedAt time.Time) (time.Time, time.Time) {
	now = now.UTC()
	if clientModifiedAt.IsZero() || clientModifiedAt.After(now.Add(maxFutureClockSkew)) {
		return now, now
	}
	return now, clientModifiedAt.UTC()
}

func (h *Handler) upsertNote(ctx context.Context, userID, clientID, guid string, req putNoteRequest, cs string, now time.Time) (model.Note, error) {
	now, req.ClientModifiedAt = utcWriteTimes(now, req.ClientModifiedAt)
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}
	var n model.Note
	err := h.DB.QueryRow(ctx,
		`INSERT INTO notes (user_id, guid, notetype_id, fields, tags, checksum,
		                    modified_at, client_modified_at, last_client_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (user_id, guid) DO UPDATE SET
		     notetype_id=EXCLUDED.notetype_id, fields=EXCLUDED.fields, tags=EXCLUDED.tags,
		     checksum=EXCLUDED.checksum, modified_at=EXCLUDED.modified_at,
		     client_modified_at=EXCLUDED.client_modified_at, last_client_id=EXCLUDED.last_client_id
		 RETURNING id, guid, notetype_id, fields, tags, checksum,
		           modified_at, client_modified_at, COALESCE(last_client_id::text,'')`,
		userID, guid, req.NotetypeID, req.Fields, tags, cs, now, req.ClientModifiedAt, clientID,
	).Scan(&n.ID, &n.GUID, &n.NotetypeID, &n.Fields, &n.Tags, &n.Checksum,
		&n.ModifiedAt, &n.ClientModifiedAt, &n.LastClientID)
	if err != nil {
		return n, err
	}
	n.UserID = userID
	h.clearTombstone(ctx, userID, "note", guid)
	return n, nil
}

func (h *Handler) upsertCard(ctx context.Context, userID, clientID string, cardID int64, req putCardRequest, now time.Time) (model.Card, error) {
	now, req.ClientModifiedAt = utcWriteTimes(now, req.ClientModifiedAt)
	sched := req.Scheduling
	if sched == nil {
		sched = map[string]any{}
	}
	var c model.Card
	// Cards are keyed by logical identity (note_guid, ord), NOT card_id, so two
	// devices that generated the same logical card with different local card_ids
	// resolve to one server row. The first writer's card_id is kept canonical on
	// conflict; only content/scheduling is updated.
	err := h.DB.QueryRow(ctx,
		`INSERT INTO cards (user_id, card_id, note_guid, deck_name, ord, scheduling,
		                    modified_at, client_modified_at, last_client_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (user_id, note_guid, ord) WHERE note_guid <> '' DO UPDATE SET
		     deck_name=EXCLUDED.deck_name,
		     scheduling=EXCLUDED.scheduling, modified_at=EXCLUDED.modified_at,
		     client_modified_at=EXCLUDED.client_modified_at, last_client_id=EXCLUDED.last_client_id
		 RETURNING id, card_id, note_guid, deck_name, ord, scheduling,
		           modified_at, client_modified_at, COALESCE(last_client_id::text,'')`,
		userID, cardID, req.NoteGUID, req.DeckName, req.Ord, sched, now, req.ClientModifiedAt, clientID,
	).Scan(&c.ID, &c.CardID, &c.NoteGUID, &c.DeckName, &c.Ord, &c.Scheduling,
		&c.ModifiedAt, &c.ClientModifiedAt, &c.LastClientID)
	if err != nil {
		return c, err
	}
	c.UserID = userID
	h.clearTombstone(ctx, userID, "card", int64ToStr(c.CardID))
	return c, nil
}

func (h *Handler) upsertNotetype(ctx context.Context, userID, clientID string, id int64, req putNotetypeRequest, cs string, now time.Time) (model.Notetype, error) {
	now, req.ClientModifiedAt = utcWriteTimes(now, req.ClientModifiedAt)
	var nt model.Notetype
	err := h.DB.QueryRow(ctx,
		`INSERT INTO notetypes (user_id, notetype_id, name, definition, checksum,
		                        modified_at, client_modified_at, last_client_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (user_id, notetype_id) DO UPDATE SET
		     name=EXCLUDED.name, definition=EXCLUDED.definition, checksum=EXCLUDED.checksum,
		     modified_at=EXCLUDED.modified_at, client_modified_at=EXCLUDED.client_modified_at,
		     last_client_id=EXCLUDED.last_client_id
		 RETURNING id, notetype_id, name, definition, checksum,
		           modified_at, client_modified_at, COALESCE(last_client_id::text,'')`,
		userID, id, req.Name, req.Definition, cs, now, req.ClientModifiedAt, clientID,
	).Scan(&nt.ID, &nt.NotetypeID, &nt.Name, &nt.Definition, &nt.Checksum,
		&nt.ModifiedAt, &nt.ClientModifiedAt, &nt.LastClientID)
	if err != nil {
		return nt, err
	}
	nt.UserID = userID
	h.clearTombstone(ctx, userID, "notetype", int64ToStr(id))
	return nt, nil
}

func (h *Handler) upsertDeck(ctx context.Context, userID, clientID, name string, req putDeckRequest, cs string, now time.Time) (model.Deck, error) {
	now, req.ClientModifiedAt = utcWriteTimes(now, req.ClientModifiedAt)
	cfg := req.Config
	if cfg == nil {
		cfg = map[string]any{}
	}
	var d model.Deck
	err := h.DB.QueryRow(ctx,
		`INSERT INTO decks (user_id, name, config, checksum,
		                    modified_at, client_modified_at, last_client_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (user_id, name) DO UPDATE SET
		     config=EXCLUDED.config, checksum=EXCLUDED.checksum, modified_at=EXCLUDED.modified_at,
		     client_modified_at=EXCLUDED.client_modified_at, last_client_id=EXCLUDED.last_client_id
		 RETURNING id, name, config, checksum,
		           modified_at, client_modified_at, COALESCE(last_client_id::text,'')`,
		userID, name, cfg, cs, now, req.ClientModifiedAt, clientID,
	).Scan(&d.ID, &d.Name, &d.Config, &d.Checksum,
		&d.ModifiedAt, &d.ClientModifiedAt, &d.LastClientID)
	if err != nil {
		return d, err
	}
	d.UserID = userID
	h.clearTombstone(ctx, userID, "deck", name)
	return d, nil
}

// clearTombstone removes any tombstone for a resource that has just been
// (re-)written, so it is no longer considered deleted.
func (h *Handler) clearTombstone(ctx context.Context, userID, typ, resourceID string) {
	_, _ = h.DB.Exec(ctx,
		`DELETE FROM tombstones WHERE user_id=$1 AND type=$2 AND resource_id=$3`,
		userID, typ, resourceID)
}
