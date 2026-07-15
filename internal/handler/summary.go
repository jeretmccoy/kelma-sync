package handler

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// These endpoints are intentionally separate from the sync manifest. Kelma
// Immersion needs small read models (deck names, ids, and current card state),
// not every collection checksum followed by batch pulls of full records.

type deckSummaryRow struct {
	Name      string `json:"name"`
	CardCount int64  `json:"card_count"`
}

type decksSummaryResponse struct {
	Decks []deckSummaryRow `json:"decks"`
}

type deckCardIDsResponse struct {
	DeckName string  `json:"deck_name"`
	CardIDs  []int64 `json:"card_ids"`
}

type cardStateSummary struct {
	CardID   int64    `json:"card_id"`
	NoteGUID string   `json:"note_guid"`
	DeckName string   `json:"deck_name"`
	Type     int64    `json:"type"`
	Queue    int64    `json:"queue"`
	Ivl      int64    `json:"ivl"`
	Fields   []string `json:"fields"`
}

type cardStatesSummaryResponse struct {
	Decks []deckSummaryRow   `json:"decks"`
	Cards []cardStateSummary `json:"cards"`
}

// GetDecksSummary returns one compact row per deck, including decks inferred
// from cards written by older clients before the explicit deck row existed.
func (h *Handler) GetDecksSummary(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	decks, err := h.loadDeckSummaries(r, claims.UserID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, decksSummaryResponse{Decks: decks})
}

// GetDeckCardIDsSummary returns only the ids in one deck. It replaces a full
// manifest plus pulling every card in the account just to filter their ids.
func (h *Handler) GetDeckCardIDsSummary(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	deckName := r.PathValue("name")
	if err := h.requireSummaryDeck(r, claims.UserID, deckName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "deck not found")
		} else {
			writeInternalError(w, err)
		}
		return
	}

	rows, err := h.DB.Query(r.Context(),
		`SELECT card_id FROM cards
		 WHERE user_id=$1 AND deck_name=$2 ORDER BY card_id`,
		claims.UserID, deckName)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			writeInternalError(w, err)
			return
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deckCardIDsResponse{DeckName: deckName, CardIDs: ids})
}

// GetDeckCardStatesSummary returns the exact text/state projection needed by
// one-deck recognition and imports, in one indexed query.
func (h *Handler) GetDeckCardStatesSummary(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	deckName := r.PathValue("name")
	if err := h.requireSummaryDeck(r, claims.UserID, deckName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "deck not found")
		} else {
			writeInternalError(w, err)
		}
		return
	}
	cards, err := h.loadCardStateSummaries(r, claims.UserID, deckName)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cardStatesSummaryResponse{
		Decks: []deckSummaryRow{},
		Cards: cards,
	})
}

// GetAllCardStatesSummary returns the card-mirror projection account-wide. It
// is still proportional to the requested summaries, but carries no manifests,
// checksums, notetype/deck definitions, or follow-up batch-pull round trips.
func (h *Handler) GetAllCardStatesSummary(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	decks, err := h.loadDeckSummaries(r, claims.UserID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	cards, err := h.loadCardStateSummaries(r, claims.UserID, "")
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cardStatesSummaryResponse{Decks: decks, Cards: cards})
}

func (h *Handler) requireSummaryDeck(r *http.Request, userID, deckName string) error {
	var exists bool
	err := h.DB.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM decks WHERE user_id=$1 AND name=$2)
		     OR EXISTS(SELECT 1 FROM cards WHERE user_id=$1 AND deck_name=$2)`,
		userID, deckName).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return pgx.ErrNoRows
	}
	return nil
}

func (h *Handler) loadDeckSummaries(r *http.Request, userID string) ([]deckSummaryRow, error) {
	rows, err := h.DB.Query(r.Context(),
		`WITH names AS (
		   SELECT name FROM decks WHERE user_id=$1
		   UNION
		   SELECT deck_name AS name FROM cards WHERE user_id=$1
		 )
		 SELECT names.name, COUNT(cards.card_id)
		 FROM names
		 LEFT JOIN cards ON cards.user_id=$1 AND cards.deck_name=names.name
		 GROUP BY names.name
		 ORDER BY lower(names.name), names.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []deckSummaryRow{}
	for rows.Next() {
		var row deckSummaryRow
		if err := rows.Scan(&row.Name, &row.CardCount); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (h *Handler) loadCardStateSummaries(r *http.Request, userID, deckName string) ([]cardStateSummary, error) {
	rows, err := h.DB.Query(r.Context(),
		`SELECT c.card_id, c.note_guid, c.deck_name,
		        COALESCE((c.scheduling->>'type')::bigint, 0),
		        COALESCE((c.scheduling->>'queue')::bigint, 0),
		        COALESCE((c.scheduling->>'ivl')::bigint, 0),
		        COALESCE(n.fields, '[]'::jsonb)
		 FROM cards c
		 LEFT JOIN notes n ON n.user_id=c.user_id AND n.guid=c.note_guid
		 WHERE c.user_id=$1 AND ($2='' OR c.deck_name=$2)
		 ORDER BY lower(c.deck_name), c.deck_name, c.card_id`,
		userID, deckName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []cardStateSummary{}
	for rows.Next() {
		var row cardStateSummary
		if err := rows.Scan(&row.CardID, &row.NoteGUID, &row.DeckName,
			&row.Type, &row.Queue, &row.Ivl, &row.Fields); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
