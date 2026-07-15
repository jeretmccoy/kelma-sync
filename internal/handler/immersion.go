package handler

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	maxImmersionMediaFiles = 25
	maxImmersionBodyBytes  = 140 << 20 // 100 MiB decoded media plus base64/JSON overhead
)

type immersionNotetype struct {
	NotetypeID int64          `json:"notetype_id"`
	Name       string         `json:"name"`
	Definition map[string]any `json:"definition"`
}

type immersionNote struct {
	GUID   string   `json:"guid"`
	Fields []string `json:"fields"`
	Tags   []string `json:"tags"`
}

type immersionCard struct {
	CardID     int64          `json:"card_id"`
	Ord        int            `json:"ord"`
	Scheduling map[string]any `json:"scheduling"`
}

type immersionMediaFile struct {
	Filename   string `json:"filename"`
	DataBase64 string `json:"data_base64"`
}

type immersionCardRequest struct {
	DeckName         string               `json:"deck_name"`
	DeckConfig       map[string]any       `json:"deck_config"`
	CreateDeck       bool                 `json:"create_deck"`
	Notetype         immersionNotetype    `json:"notetype"`
	Note             immersionNote        `json:"note"`
	Card             immersionCard        `json:"card"`
	MediaFiles       []immersionMediaFile `json:"media_files"`
	ClientModifiedAt time.Time            `json:"client_modified_at"`
}

type immersionCardResponse struct {
	DeckName      string `json:"deck_name"`
	NotetypeID    int64  `json:"notetype_id"`
	NoteGUID      string `json:"note_guid"`
	CardID        int64  `json:"card_id"`
	MediaUploaded int    `json:"media_uploaded"`
}

type preparedImmersionMedia struct {
	filename    string
	data        []byte
	storageKey  string
	contentType string
}

// PostImmersionCard is the create-only, idempotent fast path used by Kelma
// Immersion. Dependencies and the note/card are persisted in one database
// transaction, while referenced media travels in the same HTTP request. It
// replaces a full manifest scan plus deck/notetype pulls and several PUTs.
func (h *Handler) PostImmersionCard(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	r.Body = http.MaxBytesReader(w, r.Body, maxImmersionBodyBytes)
	var req immersionCardRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := validateImmersionCardRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.DeckConfig == nil {
		req.DeckConfig = map[string]any{}
	}
	if req.Notetype.Definition == nil {
		req.Notetype.Definition = map[string]any{}
	}
	if req.Note.Tags == nil {
		req.Note.Tags = []string{}
	}
	if req.Card.Scheduling == nil {
		req.Card.Scheduling = map[string]any{}
	}

	if !req.CreateDeck {
		var exists bool
		if err := h.DB.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM decks WHERE user_id=$1 AND name=$2)`,
			claims.UserID, req.DeckName).Scan(&exists); err != nil {
			writeInternalError(w, err)
			return
		}
		if !exists {
			writeError(w, http.StatusNotFound, "deck_not_found", "deck not found")
			return
		}
	}

	media, err := prepareImmersionMedia(claims.UserID, req.MediaFiles)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := h.storeImmersionMedia(r, media); err != nil {
		writeInternalError(w, err)
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck
	now, normalizedClientTime := utcWriteTimes(time.Now(), req.ClientModifiedAt)
	req.ClientModifiedAt = normalizedClientTime

	if req.CreateDeck {
		_, err = tx.Exec(r.Context(),
			`INSERT INTO decks (user_id,name,config,checksum,modified_at,client_modified_at,last_client_id)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)
			 ON CONFLICT (user_id,name) DO NOTHING`,
			claims.UserID, req.DeckName, req.DeckConfig, deckChecksum(req.DeckConfig), now,
			req.ClientModifiedAt, claims.ClientID)
		if err != nil {
			writeInternalError(w, err)
			return
		}
	}

	modelID := req.Notetype.NotetypeID
	err = tx.QueryRow(r.Context(),
		`SELECT notetype_id FROM notetypes
		 WHERE user_id=$1 AND name=$2 ORDER BY modified_at LIMIT 1`,
		claims.UserID, req.Notetype.Name).Scan(&modelID)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(r.Context(),
			`INSERT INTO notetypes (user_id,notetype_id,name,definition,checksum,modified_at,client_modified_at,last_client_id)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			 ON CONFLICT (user_id,notetype_id) DO NOTHING`,
			claims.UserID, modelID, req.Notetype.Name, req.Notetype.Definition,
			notetypeChecksum(req.Notetype.Name, req.Notetype.Definition), now,
			req.ClientModifiedAt, claims.ClientID)
	}
	if err != nil {
		writeInternalError(w, err)
		return
	}

	_, err = tx.Exec(r.Context(),
		`INSERT INTO notes (user_id,guid,notetype_id,fields,tags,checksum,modified_at,client_modified_at,last_client_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (user_id,guid) DO NOTHING`,
		claims.UserID, req.Note.GUID, modelID, req.Note.Fields, req.Note.Tags,
		checksum(req.Note.Fields, req.Note.Tags), now, req.ClientModifiedAt, claims.ClientID)
	if err != nil {
		writeInternalError(w, err)
		return
	}

	canonicalCardID := req.Card.CardID
	err = tx.QueryRow(r.Context(),
		`INSERT INTO cards (user_id,card_id,note_guid,deck_name,ord,scheduling,modified_at,client_modified_at,last_client_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (user_id,note_guid,ord) WHERE note_guid <> '' DO NOTHING
		 RETURNING card_id`,
		claims.UserID, req.Card.CardID, req.Note.GUID, req.DeckName, req.Card.Ord,
		req.Card.Scheduling, now, req.ClientModifiedAt, claims.ClientID).Scan(&canonicalCardID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(r.Context(),
			`SELECT card_id FROM cards WHERE user_id=$1 AND note_guid=$2 AND ord=$3`,
			claims.UserID, req.Note.GUID, req.Card.Ord).Scan(&canonicalCardID)
	}
	if err != nil {
		writeInternalError(w, err)
		return
	}

	for _, file := range media {
		_, err = tx.Exec(r.Context(),
			`INSERT INTO media (user_id,filename,size_bytes,storage_key,modified_at)
			 VALUES ($1,$2,$3,$4,$5)
			 ON CONFLICT (user_id,filename) DO UPDATE SET
			   size_bytes=EXCLUDED.size_bytes, storage_key=EXCLUDED.storage_key,
			   modified_at=EXCLUDED.modified_at`,
			claims.UserID, file.filename, len(file.data), file.storageKey, now)
		if err != nil {
			writeInternalError(w, err)
			return
		}
	}

	_, err = tx.Exec(r.Context(),
		`DELETE FROM tombstones WHERE user_id=$1 AND (
		   (type='deck' AND resource_id=$2) OR
		   (type='notetype' AND resource_id=$3) OR
		   (type='note' AND resource_id=$4) OR
		   (type='card' AND resource_id=$5) OR
		   (type='media' AND resource_id=ANY($6::text[]))
		 )`, claims.UserID, req.DeckName, int64ToStr(modelID), req.Note.GUID,
		int64ToStr(canonicalCardID), immersionMediaNames(media))
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeInternalError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, immersionCardResponse{
		DeckName: req.DeckName, NotetypeID: modelID, NoteGUID: req.Note.GUID,
		CardID: canonicalCardID, MediaUploaded: len(media),
	})
}

func validateImmersionCardRequest(req immersionCardRequest) error {
	if strings.TrimSpace(req.DeckName) == "" {
		return fmt.Errorf("deck_name is required")
	}
	if req.Notetype.NotetypeID == 0 || strings.TrimSpace(req.Notetype.Name) == "" {
		return fmt.Errorf("notetype id and name are required")
	}
	if strings.TrimSpace(req.Note.GUID) == "" || len(req.Note.Fields) == 0 || strings.TrimSpace(req.Note.Fields[0]) == "" {
		return fmt.Errorf("note guid and front field are required")
	}
	if req.Card.CardID == 0 {
		return fmt.Errorf("card_id is required")
	}
	if len(req.MediaFiles) > maxImmersionMediaFiles {
		return fmt.Errorf("media_files is limited to %d files", maxImmersionMediaFiles)
	}
	return nil
}

func prepareImmersionMedia(userID string, files []immersionMediaFile) ([]preparedImmersionMedia, error) {
	out := make([]preparedImmersionMedia, 0, len(files))
	total := 0
	seen := map[string]bool{}
	for _, file := range files {
		filename := strings.TrimSpace(file.Filename)
		if filename == "" || len(filename) > 255 || strings.ContainsAny(filename, `/\\`) {
			return nil, fmt.Errorf("invalid media filename")
		}
		if seen[filename] {
			return nil, fmt.Errorf("duplicate media filename %s", filename)
		}
		seen[filename] = true
		data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(file.DataBase64))
		if err != nil || len(data) == 0 {
			return nil, fmt.Errorf("media file %s was not valid base64", filename)
		}
		total += len(data)
		if total > maxMediaBytes {
			return nil, fmt.Errorf("combined media exceeds %d bytes", maxMediaBytes)
		}
		contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		out = append(out, preparedImmersionMedia{
			filename: filename, data: data, storageKey: storageKey(userID, filename), contentType: contentType,
		})
	}
	return out, nil
}

func (h *Handler) storeImmersionMedia(r *http.Request, files []preparedImmersionMedia) error {
	if len(files) == 0 {
		return nil
	}
	errs := make(chan error, len(files))
	var wg sync.WaitGroup
	for _, source := range files {
		file := source
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.Storage.Put(r.Context(), file.storageKey, bytes.NewReader(file.data), int64(len(file.data)), file.contentType); err != nil {
				errs <- fmt.Errorf("media %s: storage write failed", file.filename)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func immersionMediaNames(files []preparedImmersionMedia) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		out = append(out, file.filename)
	}
	return out
}
