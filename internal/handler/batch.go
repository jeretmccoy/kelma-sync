package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jeretmccoy/kelma-sync/internal/model"
)

type batchNote struct {
	GUID string `json:"guid"`
	putNoteRequest
}
type batchCard struct {
	CardID int64 `json:"card_id"`
	putCardRequest
}
type batchNotetype struct {
	NotetypeID int64 `json:"notetype_id"`
	putNotetypeRequest
}
type batchDeck struct {
	Name string `json:"name"`
	putDeckRequest
}

type batchPushRequest struct {
	Notes     []batchNote          `json:"notes"`
	Cards     []batchCard          `json:"cards"`
	Reviews   []putReviewRequest   `json:"reviews"`
	StudyDays []putStudyDayRequest `json:"study_days"`
	Notetypes []batchNotetype      `json:"notetypes"`
	Decks     []batchDeck          `json:"decks"`
}

type conflictEntry struct {
	GUID       string `json:"guid,omitempty"`
	ReviewID   int64  `json:"review_id,omitempty"`
	NotetypeID int64  `json:"notetype_id,omitempty"`
	Name       string `json:"name,omitempty"`
	Server     any    `json:"server"`
	Client     any    `json:"client"`
}

type batchPushResponse struct {
	Accepted  map[string]int             `json:"accepted"`
	Conflicts map[string][]conflictEntry `json:"conflicts"`
}

// BatchPush accepts many records at once. Notes, notetypes, and decks that
// conflict are returned for resolution; everything else is written. Cards use
// silent newest-wins. Force-Override accepts everything unconditionally.
func (h *Handler) BatchPush(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	ctx := r.Context()
	force := forceOverride(r)

	var req batchPushRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	resp := batchPushResponse{
		Accepted: map[string]int{
			"notes": 0, "cards": 0, "reviews": 0, "study_days": 0,
			"notetypes": 0, "decks": 0,
		},
		Conflicts: map[string][]conflictEntry{
			"notes": {}, "reviews": {}, "notetypes": {}, "decks": {},
		},
	}
	now := time.Now().UTC()

	// Precompute all note checksums in one batch (one binary spawn when the
	// Rust hasher is enabled), instead of one call per note.
	noteItems := make([][]any, len(req.Notes))
	for i, n := range req.Notes {
		tags := n.Tags
		if tags == nil {
			tags = []string{}
		}
		noteItems[i] = []any{n.Fields, tags}
	}
	noteChecksums := checksumBatch(noteItems)

	for i, n := range req.Notes {
		if n.Tags == nil {
			n.Tags = []string{}
		}
		cs := noteChecksums[i]
		existing, err := h.loadNote(r, claims.UserID, n.GUID)
		notFound := errors.Is(err, pgx.ErrNoRows)
		if err != nil && !notFound {
			writeInternalError(w, err)
			return
		}
		if !force && !notFound && existing.Checksum != n.BaseChecksum && existing.Checksum != cs {
			resp.Conflicts["notes"] = append(resp.Conflicts["notes"],
				conflictEntry{GUID: n.GUID, Server: existing, Client: n})
			continue
		}
		if _, err := h.upsertNote(ctx, claims.UserID, claims.ClientID, n.GUID, n.putNoteRequest, cs, now); err != nil {
			writeInternalError(w, err)
			return
		}
		resp.Accepted["notes"]++
	}

	for _, c := range req.Cards {
		_, c.ClientModifiedAt = utcWriteTimes(now, c.ClientModifiedAt)
		if c.Scheduling == nil {
			c.Scheduling = map[string]any{}
		}
		if c.NoteGUID == "" {
			continue // blank GUID is not a valid logical card identity
		}
		// Resolve by logical identity (note_guid, ord), not card_id.
		existing, err := h.loadCardLogical(r, claims.UserID, c.NoteGUID, c.Ord)
		notFound := errors.Is(err, pgx.ErrNoRows)
		if err != nil && !notFound {
			writeInternalError(w, err)
			return
		}
		if !notFound && existing.ClientModifiedAt.After(c.ClientModifiedAt) {
			continue // silent newest-wins
		}
		cardID := c.CardID
		if !notFound {
			cardID = existing.CardID // keep canonical id
		}
		if _, err := h.upsertCard(ctx, claims.UserID, claims.ClientID, cardID, c.putCardRequest, now); err != nil {
			writeInternalError(w, err)
			return
		}
		resp.Accepted["cards"]++
	}

	for _, review := range req.Reviews {
		if review.ReviewID <= 0 {
			writeError(w, http.StatusBadRequest, "bad_review_history", "review_id must be positive")
			return
		}
	}
	reviewCollisions, err := h.insertReviews(
		ctx, claims.UserID, claims.ClientID, req.Reviews, now,
	)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	for _, collision := range reviewCollisions {
		resp.Conflicts["reviews"] = append(resp.Conflicts["reviews"],
			conflictEntry{
				ReviewID: collision.Client.ReviewID,
				Server:   collision.Server,
				Client:   collision.Client,
			})
	}
	resp.Accepted["reviews"] = len(req.Reviews) - len(reviewCollisions)

	for _, day := range req.StudyDays {
		if _, err := h.upsertStudyDay(ctx, claims.UserID, claims.ClientID, day, now); err != nil {
			writeError(w, http.StatusBadRequest, "bad_study_day", err.Error())
			return
		}
		resp.Accepted["study_days"]++
	}

	for _, nt := range req.Notetypes {
		cs := notetypeChecksum(nt.Name, nt.Definition)
		existing, err := h.loadNotetype(r, claims.UserID, nt.NotetypeID)
		notFound := errors.Is(err, pgx.ErrNoRows)
		if err != nil && !notFound {
			writeInternalError(w, err)
			return
		}
		if !force && !notFound && existing.Checksum != nt.BaseChecksum && existing.Checksum != cs {
			resp.Conflicts["notetypes"] = append(resp.Conflicts["notetypes"],
				conflictEntry{NotetypeID: nt.NotetypeID, Server: existing, Client: nt})
			continue
		}
		if _, err := h.upsertNotetype(ctx, claims.UserID, claims.ClientID, nt.NotetypeID, nt.putNotetypeRequest, cs, now); err != nil {
			writeInternalError(w, err)
			return
		}
		resp.Accepted["notetypes"]++
	}

	for _, d := range req.Decks {
		if d.Config == nil {
			d.Config = map[string]any{}
		}
		cs := deckChecksum(d.Config)
		existing, err := h.loadDeck(r, claims.UserID, d.Name)
		notFound := errors.Is(err, pgx.ErrNoRows)
		if err != nil && !notFound {
			writeInternalError(w, err)
			return
		}
		if !force && !notFound && existing.Checksum != d.BaseChecksum && existing.Checksum != cs {
			resp.Conflicts["decks"] = append(resp.Conflicts["decks"],
				conflictEntry{Name: d.Name, Server: existing, Client: d})
			continue
		}
		if _, err := h.upsertDeck(ctx, claims.UserID, claims.ClientID, d.Name, d.putDeckRequest, cs, now); err != nil {
			writeInternalError(w, err)
			return
		}
		resp.Accepted["decks"]++
	}

	writeJSON(w, http.StatusOK, resp)
}

type batchPullRequest struct {
	Notes     []string `json:"notes"`
	Cards     []int64  `json:"cards"`
	Reviews   []int64  `json:"reviews"`
	Notetypes []int64  `json:"notetypes"`
	Decks     []string `json:"decks"`
}

type batchPullResponse struct {
	Notes     []model.Note     `json:"notes"`
	Cards     []model.Card     `json:"cards"`
	Reviews   []model.Review   `json:"reviews"`
	Notetypes []model.Notetype `json:"notetypes"`
	Decks     []model.Deck     `json:"decks"`
}

// BatchPull returns full records for the requested identifiers.
type batchDeleteRequest struct {
	Notes     []string `json:"notes"`
	Cards     []int64  `json:"cards"`
	Notetypes []int64  `json:"notetypes"`
	Decks     []string `json:"decks"`
}

type batchDeleteResponse struct {
	Requested map[string]int `json:"requested"`
	Deleted   map[string]int `json:"deleted"`
}

// BatchDelete removes many resources transactionally and writes matching
// tombstones. The client plans deletion from a scope-bound local snapshot; the
// server limits request size as a final defense against accidental mass wipes.
func (h *Handler) BatchDelete(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	var req batchDeleteRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	total := len(req.Notes) + len(req.Cards) + len(req.Notetypes) + len(req.Decks)
	if total == 0 {
		writeJSON(w, http.StatusOK, batchDeleteResponse{
			Requested: map[string]int{"notes": 0, "cards": 0, "notetypes": 0, "decks": 0},
			Deleted:   map[string]int{"notes": 0, "cards": 0, "notetypes": 0, "decks": 0},
		})
		return
	}
	if total > 12000 {
		writeError(w, http.StatusBadRequest, "too_many_deletes", "batch delete is limited to 12000 resources")
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	deleted := map[string]int{"notes": 0, "cards": 0, "notetypes": 0, "decks": 0}
	types := []struct {
		name      string
		values    any
		deleteSQL string
		stoneSQL  string
	}{
		{"notes", req.Notes,
			`DELETE FROM notes WHERE user_id=$1 AND guid=ANY($2::text[])`,
			`INSERT INTO tombstones(user_id,type,resource_id,last_client_id)
			 SELECT $1,'note',value,$3 FROM unnest($2::text[]) value
			 ON CONFLICT(user_id,type,resource_id) DO UPDATE SET deleted_at=now(),last_client_id=EXCLUDED.last_client_id`},
		{"cards", req.Cards,
			`DELETE FROM cards WHERE user_id=$1 AND card_id=ANY($2::bigint[])`,
			`INSERT INTO tombstones(user_id,type,resource_id,last_client_id)
			 SELECT $1,'card',value::text,$3 FROM unnest($2::bigint[]) value
			 ON CONFLICT(user_id,type,resource_id) DO UPDATE SET deleted_at=now(),last_client_id=EXCLUDED.last_client_id`},
		{"notetypes", req.Notetypes,
			`DELETE FROM notetypes WHERE user_id=$1 AND notetype_id=ANY($2::bigint[])`,
			`INSERT INTO tombstones(user_id,type,resource_id,last_client_id)
			 SELECT $1,'notetype',value::text,$3 FROM unnest($2::bigint[]) value
			 ON CONFLICT(user_id,type,resource_id) DO UPDATE SET deleted_at=now(),last_client_id=EXCLUDED.last_client_id`},
		{"decks", req.Decks,
			`DELETE FROM decks WHERE user_id=$1 AND name=ANY($2::text[])`,
			`INSERT INTO tombstones(user_id,type,resource_id,last_client_id)
			 SELECT $1,'deck',value,$3 FROM unnest($2::text[]) value
			 ON CONFLICT(user_id,type,resource_id) DO UPDATE SET deleted_at=now(),last_client_id=EXCLUDED.last_client_id`},
	}
	for _, item := range types {
		tag, err := tx.Exec(r.Context(), item.deleteSQL, claims.UserID, item.values)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		deleted[item.name] = int(tag.RowsAffected())
		if _, err := tx.Exec(r.Context(), item.stoneSQL, claims.UserID, item.values, claims.ClientID); err != nil {
			writeInternalError(w, err)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, batchDeleteResponse{
		Requested: map[string]int{"notes": len(req.Notes), "cards": len(req.Cards), "notetypes": len(req.Notetypes), "decks": len(req.Decks)},
		Deleted:   deleted,
	})
}

func (h *Handler) BatchPull(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	var req batchPullRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	resp := batchPullResponse{
		Notes: []model.Note{}, Cards: []model.Card{}, Reviews: []model.Review{},
		Notetypes: []model.Notetype{}, Decks: []model.Deck{},
	}
	for _, guid := range req.Notes {
		if n, err := h.loadNote(r, claims.UserID, guid); err == nil {
			resp.Notes = append(resp.Notes, n)
		}
	}
	for _, id := range req.Cards {
		if c, err := h.loadCard(r, claims.UserID, id); err == nil {
			resp.Cards = append(resp.Cards, c)
		}
	}
	if reviews, err := h.loadReviews(r.Context(), claims.UserID, req.Reviews); err == nil {
		resp.Reviews = reviews
	} else {
		writeInternalError(w, err)
		return
	}
	for _, id := range req.Notetypes {
		if nt, err := h.loadNotetype(r, claims.UserID, id); err == nil {
			resp.Notetypes = append(resp.Notetypes, nt)
		}
	}
	for _, name := range req.Decks {
		if d, err := h.loadDeck(r, claims.UserID, name); err == nil {
			resp.Decks = append(resp.Decks, d)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
