package handler

import "net/http"

type usageResponse struct {
	UsedBytes    int64 `json:"used_bytes"`
	MediaBytes   int64 `json:"media_bytes"`
	ContentBytes int64 `json:"content_bytes"`
}

// GetUsage reports the authenticated user's exact KelmaSync storage usage.
// It is intentionally account-wide: routing controls what this client uploads,
// while the server total includes content retained by every KelmaSync client.
func (h *Handler) GetUsage(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	var mediaBytes, contentBytes int64
	if err := h.DB.QueryRow(r.Context(), `
		SELECT COALESCE((SELECT SUM(size_bytes) FROM media WHERE user_id = $1), 0),
		       COALESCE((SELECT SUM(octet_length(fields::text) + octet_length(tags::text)) FROM notes WHERE user_id = $1), 0)
		     + COALESCE((SELECT SUM(octet_length(scheduling::text)) FROM cards WHERE user_id = $1), 0)
		     + COALESCE((SELECT SUM(octet_length(definition::text)) FROM notetypes WHERE user_id = $1), 0)
		     + COALESCE((SELECT SUM(octet_length(config::text)) FROM decks WHERE user_id = $1), 0)
		     + COALESCE((SELECT SUM(
		           8 + 8 + octet_length(note_guid) + 4 + octet_length(deck_name)
		           + 2 + 4 + 4 + 4 + 4 + 2
		       ) FROM reviews WHERE user_id = $1), 0)
		     + COALESCE((SELECT COUNT(*) * (8 + 4 + 4 + 4 + 8) + SUM(octet_length(deck_name))
		       FROM study_days WHERE user_id = $1), 0)
	`, claims.UserID).Scan(&mediaBytes, &contentBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "usage query failed")
		return
	}
	writeJSON(w, http.StatusOK, usageResponse{
		UsedBytes:    mediaBytes + contentBytes,
		MediaBytes:   mediaBytes,
		ContentBytes: contentBytes,
	})
}
