package handler

import "net/http"

// writeTombstone records a deletion so other clients remove the resource rather
// than re-pushing it. Upserts on (user_id, type, resource_id).
func (h *Handler) writeTombstone(r *http.Request, userID, typ, resourceID, clientID string) error {
	_, err := h.DB.Exec(r.Context(),
		`INSERT INTO tombstones (user_id, type, resource_id, deleted_at, last_client_id)
		 VALUES ($1, $2, $3, now(), $4)
		 ON CONFLICT (user_id, type, resource_id)
		 DO UPDATE SET deleted_at = now(), last_client_id = EXCLUDED.last_client_id`,
		userID, typ, resourceID, clientID)
	return err
}
