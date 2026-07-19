package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jeretmccoy/kelma-sync/internal/model"
)

// putReviewRequest is an immutable Anki revlog row. ReviewID is Anki's
// millisecond timestamp id; NoteGUID + CardOrd allow destination collections to
// replace SourceCardID with their own local card id.
type putReviewRequest struct {
	ReviewID     int64  `json:"review_id"`
	SourceCardID int64  `json:"source_card_id"`
	NoteGUID     string `json:"note_guid"`
	CardOrd      int    `json:"card_ord"`
	DeckName     string `json:"deck_name"`
	Ease         int    `json:"ease"`
	Interval     int    `json:"interval"`
	LastInterval int    `json:"last_interval"`
	Factor       int    `json:"factor"`
	TakenMillis  int    `json:"taken_millis"`
	ReviewKind   int    `json:"review_kind"`
}

type putStudyDayRequest struct {
	Day                 int64  `json:"day"`
	DeckName            string `json:"deck_name"`
	NewStudied          int    `json:"new_studied"`
	ReviewStudied       int    `json:"review_studied"`
	LearningStudied     int    `json:"learning_studied"`
	MillisecondsStudied int64  `json:"milliseconds_studied"`
}

type reviewCollisionError struct {
	Server model.Review
	Client putReviewRequest
}

func (e *reviewCollisionError) Error() string {
	return fmt.Sprintf("review id %d has different immutable content", e.Client.ReviewID)
}

func checksumForReview(req putReviewRequest) string {
	return reviewChecksum(
		req.NoteGUID,
		req.CardOrd,
		req.Ease,
		req.Interval,
		req.LastInterval,
		req.Factor,
		req.TakenMillis,
		req.ReviewKind,
	)
}

func (h *Handler) insertReviews(
	ctx context.Context,
	userID, clientID string,
	requests []putReviewRequest,
	now time.Time,
) ([]reviewCollisionError, error) {
	if len(requests) == 0 {
		return nil, nil
	}
	type insertRow struct {
		putReviewRequest
		Checksum string `json:"checksum"`
	}
	rows := make([]insertRow, len(requests))
	ids := make([]int64, len(requests))
	for i, request := range requests {
		if request.ReviewID <= 0 {
			return nil, fmt.Errorf("review_id must be positive")
		}
		rows[i] = insertRow{putReviewRequest: request, Checksum: checksumForReview(request)}
		ids[i] = request.ReviewID
	}
	payload, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	_, err = h.DB.Exec(ctx,
		`INSERT INTO reviews (
		     user_id, review_id, source_card_id, note_guid, card_ord, deck_name,
		     ease, interval, last_interval, factor, taken_millis, review_kind,
		     checksum, modified_at, last_client_id
		 )
		 SELECT $1, incoming.review_id, incoming.source_card_id,
		        incoming.note_guid, incoming.card_ord, incoming.deck_name,
		        incoming.ease, incoming.interval, incoming.last_interval,
		        incoming.factor, incoming.taken_millis, incoming.review_kind,
		        incoming.checksum, $3, $4
		 FROM jsonb_to_recordset($2::jsonb) AS incoming(
		     review_id bigint, source_card_id bigint, note_guid text,
		     card_ord int, deck_name text, ease int, interval int,
		     last_interval int, factor int, taken_millis int,
		     review_kind int, checksum text
		 )
		 ON CONFLICT (user_id, review_id) DO NOTHING`,
		userID, payload, now.UTC(), clientID,
	)
	if err != nil {
		return nil, err
	}

	stored := map[int64]string{}
	queryRows, err := h.DB.Query(ctx,
		`SELECT review_id, checksum FROM reviews
		 WHERE user_id=$1 AND review_id=ANY($2::bigint[])`,
		userID, ids,
	)
	if err != nil {
		return nil, err
	}
	for queryRows.Next() {
		var id int64
		var checksum string
		if err := queryRows.Scan(&id, &checksum); err != nil {
			queryRows.Close()
			return nil, err
		}
		stored[id] = checksum
	}
	queryRows.Close()
	if err := queryRows.Err(); err != nil {
		return nil, err
	}

	var collisions []reviewCollisionError
	for i, request := range requests {
		if stored[request.ReviewID] == rows[i].Checksum {
			continue
		}
		server, err := h.loadReview(ctx, userID, request.ReviewID)
		if err != nil {
			return nil, err
		}
		collisions = append(collisions, reviewCollisionError{
			Server: server,
			Client: request,
		})
	}
	return collisions, nil
}

func (h *Handler) loadReview(
	ctx context.Context, userID string, reviewID int64,
) (model.Review, error) {
	var review model.Review
	err := h.DB.QueryRow(ctx,
		`SELECT r.id, r.user_id, r.review_id, r.source_card_id, r.note_guid,
		        r.card_ord, r.deck_name, r.ease, r.interval, r.last_interval,
		        r.factor, r.taken_millis, r.review_kind, r.checksum, r.modified_at,
		        COALESCE(r.last_client_id::text, ''), COALESCE(c.label, '')
		 FROM reviews r LEFT JOIN clients c ON c.id = r.last_client_id
		 WHERE r.user_id=$1 AND r.review_id=$2`,
		userID, reviewID,
	).Scan(
		&review.ID, &review.UserID, &review.ReviewID, &review.SourceCardID,
		&review.NoteGUID, &review.CardOrd, &review.DeckName, &review.Ease,
		&review.Interval, &review.LastInterval, &review.Factor,
		&review.TakenMillis, &review.ReviewKind, &review.Checksum,
		&review.ModifiedAt, &review.LastClientID, &review.LastClientLabel,
	)
	if err == nil {
		review.ModifiedAt = review.ModifiedAt.UTC()
	}
	return review, err
}

// upsertStudyDay monotonically merges an append-only day's counters. This is a
// portable quota snapshot, not the source of review history; reviews remain the
// authoritative event log.
func (h *Handler) upsertStudyDay(
	ctx context.Context,
	userID, clientID string,
	req putStudyDayRequest,
	now time.Time,
) (model.StudyDay, error) {
	if req.DeckName == "" {
		return model.StudyDay{}, fmt.Errorf("study day deck_name is required")
	}
	var day model.StudyDay
	err := h.DB.QueryRow(ctx,
		`INSERT INTO study_days (
		     user_id, day, deck_name, new_studied, review_studied,
		     learning_studied, milliseconds_studied, modified_at, last_client_id
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (user_id, day, deck_name) DO UPDATE SET
		     new_studied=GREATEST(study_days.new_studied, EXCLUDED.new_studied),
		     review_studied=GREATEST(study_days.review_studied, EXCLUDED.review_studied),
		     learning_studied=GREATEST(study_days.learning_studied, EXCLUDED.learning_studied),
		     milliseconds_studied=GREATEST(study_days.milliseconds_studied, EXCLUDED.milliseconds_studied),
		     modified_at=EXCLUDED.modified_at,
		     last_client_id=EXCLUDED.last_client_id
		 RETURNING id, user_id, day, deck_name, new_studied, review_studied,
		           learning_studied, milliseconds_studied, modified_at,
		           COALESCE(last_client_id::text, '')`,
		userID, req.Day, req.DeckName, req.NewStudied, req.ReviewStudied,
		req.LearningStudied, req.MillisecondsStudied, now.UTC(), clientID,
	).Scan(
		&day.ID, &day.UserID, &day.Day, &day.DeckName, &day.NewStudied,
		&day.ReviewStudied, &day.LearningStudied, &day.MillisecondsStudied,
		&day.ModifiedAt, &day.LastClientID,
	)
	day.ModifiedAt = day.ModifiedAt.UTC()
	return day, err
}

func (h *Handler) loadReviews(
	ctx context.Context, userID string, reviewIDs []int64,
) ([]model.Review, error) {
	if len(reviewIDs) == 0 {
		return []model.Review{}, nil
	}
	rows, err := h.DB.Query(ctx,
		`SELECT r.id, r.user_id, r.review_id, r.source_card_id, r.note_guid,
		        r.card_ord, r.deck_name, r.ease, r.interval, r.last_interval,
		        r.factor, r.taken_millis, r.review_kind, r.checksum, r.modified_at,
		        COALESCE(r.last_client_id::text, ''), COALESCE(c.label, '')
		 FROM reviews r LEFT JOIN clients c ON c.id=r.last_client_id
		 WHERE r.user_id=$1 AND r.review_id=ANY($2::bigint[])
		 ORDER BY r.review_id`,
		userID, reviewIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Review{}
	for rows.Next() {
		var review model.Review
		if err := rows.Scan(
			&review.ID, &review.UserID, &review.ReviewID,
			&review.SourceCardID, &review.NoteGUID, &review.CardOrd,
			&review.DeckName, &review.Ease, &review.Interval,
			&review.LastInterval, &review.Factor, &review.TakenMillis,
			&review.ReviewKind, &review.Checksum, &review.ModifiedAt,
			&review.LastClientID, &review.LastClientLabel,
		); err != nil {
			return nil, err
		}
		review.ModifiedAt = review.ModifiedAt.UTC()
		out = append(out, review)
	}
	return out, rows.Err()
}
