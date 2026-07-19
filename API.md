# KelmaSync v2 — API

REST over HTTPS. All request and response bodies are JSON. All endpoints except
`/v2/auth/*` require `Authorization: Bearer <token>`.

---

## Auth

### Register
```
POST /v2/auth/register
Body: { username, password }
Response 201: { user_id }
```

### Login
```
POST /v2/auth/login
Body: { username, password, client_label }
Response 200: { token, client_id }
```

`client_label` is the human-readable device name ("iPhone 15"). If a client
with that label already exists for the user, the existing `client_id` is
returned and `last_seen` is updated. Otherwise a new client record is created.

### Logout
```
POST /v2/auth/logout
Response 204
```

Revokes the token used to make the request.

---

## Sync

The sync flow is always:

1. Client calls `GET /v2/sync/manifest` to find out what changed on the server.
2. Client compares to its local state, classifies each item.
3. Client pulls items the server has that it doesn't (or that differ).
4. Client pushes items it has that the server doesn't (or that differ).
5. Conflicts (both sides changed) are held for user resolution.

### Manifest
```
GET /v2/sync/manifest?since=<iso-timestamp>
Response 200:
{
  "notes":     [{ "guid":        string, "checksum": string, "modified_at": string, "client_modified_at": string }],
  "cards":     [{ "card_id":     number, "checksum": string, "modified_at": string, "client_modified_at": string }],
  "reviews":   [{ "review_id":   number, "checksum": string, "deck_name": string, "modified_at": string }],
  "study_days":[{ "day": number, "deck_name": string, "new_studied": number, "review_studied": number, "learning_studied": number, "milliseconds_studied": number, "modified_at": string }],
  "notetypes": [{ "notetype_id": number, "checksum": string, "modified_at": string, "client_modified_at": string }],
  "decks":     [{ "name":        string, "checksum": string, "modified_at": string, "client_modified_at": string }],
  "media":     [{ "filename":    string, "modified_at": string }],
  "tombstones":[{ "type": string, "resource_id": string, "deleted_at": string }],
  "server_time": string   -- use as `since` on the next sync
}
```

Omit `since` on first sync to receive everything. `modified_at` is when the
server accepted the write; `client_modified_at` is the source collection's
mtime and lets two-source clients select a uniquely newer copy. `server_time`
is the timestamp the server used to filter this response; the client stores it
and passes it as `since` next time. Every timestamp is serialized as RFC3339
UTC (`...Z`). Clients also use `server_time` as their authoritative UTC clock;
a source clock more than five minutes in the future is clamped by the server
and must not win client-side reconciliation.

### Storage usage
```
GET /v2/usage
Response 200:
{
  "used_bytes": number,
  "media_bytes": number,
  "content_bytes": number
}
```

Returns the authenticated account's exact KelmaSync storage usage. This is an
account-wide server total, independent of the requesting client's local files.

### Kelma Immersion summaries

These read models are intentionally separate from the sync manifest. They return
only the projection the website needs and never require a follow-up collection
pull.

```
GET /v2/summary/decks
Response 200: { "decks": [{ "name": string, "card_count": number }] }
```

```
GET /v2/summary/decks/:name/card-ids
Response 200: { "deck_name": string, "card_ids": [number] }
```

```
GET /v2/summary/decks/:name/card-states
GET /v2/summary/card-states
Response 200: {
  "decks": [{ "name": string, "card_count": number }],
  "cards": [{
    "card_id": number, "note_guid": string, "deck_name": string,
    "type": number, "queue": number, "ivl": number, "fields": [string]
  }]
}
```

The deck-scoped response leaves `decks` empty because the requested deck name is
already in the URL. The account-wide response includes deck rows.

### Kelma Immersion card creation

```
POST /v2/immersion/cards
Body: {
  "deck_name": string,
  "deck_config": object,
  "create_deck": boolean,
  "notetype": { "notetype_id": number, "name": string, "definition": object },
  "note": { "guid": string, "fields": [string], "tags": [string] },
  "card": { "card_id": number, "ord": number, "scheduling": object },
  "media_files": [{ "filename": string, "data_base64": string }],
  "client_modified_at": string
}
Response 201: {
  "deck_name": string, "notetype_id": number, "note_guid": string,
  "card_id": number, "media_uploaded": number
}
```

This is a create-only, idempotent fast path. It uploads the card and media in one
HTTP request, creates missing deck/notetype dependencies, and commits the
content records in one database transaction. Reposting the same `note.guid`
returns the existing logical card instead of creating a duplicate.

---

## Notes

### Get one
```
GET /v2/notes/:guid
Response 200:
{
  "guid":                string,
  "notetype_id":         number,
  "fields":              string[],
  "tags":                string[],
  "checksum":            string,
  "modified_at":         string,
  "client_modified_at":  string,
  "last_client_id":      string,
  "last_client_label":   string
}
Response 404: { "error": "not found" }
```

### Push one
```
PUT /v2/notes/:guid
Headers (optional): Force-Override: true
Body:
{
  "notetype_id":         number,
  "fields":              string[],
  "tags":                string[],
  "client_modified_at":  string
}
Response 200: { ...saved record }
Response 409:
{
  "error": "conflict",
  "server": { ...server record },
  "client": { ...what you sent }
}
```

`409` is returned when the note exists on the server and its checksum differs
from what the client last saw. `Force-Override: true` bypasses the conflict
check and overwrites unconditionally.

### Delete one
```
DELETE /v2/notes/:guid
Response 204
```

Writes a tombstone. Idempotent.

---

## Cards

### Get one
```
GET /v2/cards/:card_id
Response 200:
{
  "card_id":             number,
  "note_guid":           string,
  "deck_name":           string,
  "ord":                 number,
  "scheduling":          object,
  "modified_at":         string,
  "client_modified_at":  string,
  "last_client_id":      string,
  "last_client_label":   string
}
```

### Push one
```
PUT /v2/cards/:card_id
Body:
{
  "note_guid":           string,
  "deck_name":           string,
  "ord":                 number,
  "scheduling":          object,
  "client_modified_at":  string
}
Response 200: { ...saved record }
```

Cards use per-card timestamp comparison, not checksum conflict detection.
The server accepts the push only if `client_modified_at` is newer than the
stored `client_modified_at`. If not, the server returns the existing record
with `200` and the client discards its local change.

### Delete one
```
DELETE /v2/cards/:card_id
Response 204
```

---

## Review history

Review history uses Anki's append-only `revlog` semantics. Rows are transferred
through the batch endpoints and identified by their millisecond `review_id`.
They are never deleted by note/card tombstones (matching native Anki sync).

A full review record is:
```
{
  "review_id": number,
  "source_card_id": number,
  "note_guid": string,
  "card_ord": number,
  "deck_name": string,
  "ease": number,
  "interval": number,
  "last_interval": number,
  "factor": number,
  "taken_millis": number,
  "review_kind": number,
  "checksum": string,
  "modified_at": string
}
```

`source_card_id` is diagnostic/fallback metadata. Card ids are local to each
collection, so clients normally map each row through `(note_guid, card_ord)`
before inserting it. Re-sending identical immutable content is idempotent. If
the same `review_id` carries different immutable content, batch push returns a
review conflict rather than silently losing history.

Anki's daily limits are stored outside `revlog` in collection-relative deck
counters. `study_days` carries their portable epoch-day snapshots. Same-day
server counters merge monotonically so an older device cannot reopen a quota
already consumed on another device.

---

## Notetypes

### Get one
```
GET /v2/notetypes/:notetype_id
Response 200:
{
  "notetype_id":         number,
  "name":                string,
  "definition":          object,
  "checksum":            string,
  "modified_at":         string,
  "client_modified_at":  string,
  "last_client_id":      string,
  "last_client_label":   string
}
```

### Push one
```
PUT /v2/notetypes/:notetype_id
Headers (optional): Force-Override: true
Body:
{
  "name":                string,
  "definition":          object,
  "client_modified_at":  string
}
Response 200: { ...saved record }
Response 409:
{
  "error": "conflict",
  "server": { ...server record },
  "client": { ...what you sent }
}
```

Same conflict semantics as notes. `Force-Override: true` overwrites
unconditionally.

### Delete one
```
DELETE /v2/notetypes/:notetype_id
Response 204
```

---

## Decks

### Get one
```
GET /v2/decks/:name
Response 200:
{
  "name":                string,
  "config":              object,
  "checksum":            string,
  "modified_at":         string,
  "client_modified_at":  string,
  "last_client_id":      string,
  "last_client_label":   string
}
```

`:name` is URL-encoded (e.g. `Japanese%3A%3AVocab`).

### Push one
```
PUT /v2/decks/:name
Headers (optional): Force-Override: true
Body:
{
  "config":              object,
  "client_modified_at":  string
}
Response 200: { ...saved record }
Response 409: { "error": "conflict", "server": {...}, "client": {...} }
```

### Delete one
```
DELETE /v2/decks/:name
Response 204
```

---

## Media

### Check presence
```
HEAD /v2/media/:filename
Response 200   -- server has it
Response 404   -- server does not have it
```

### Download
```
GET /v2/media/:filename
Response 200: <binary>
Content-Type: set to the file's actual MIME type
```

### Upload
```
PUT /v2/media/:filename
Content-Type: application/octet-stream
Body: <binary>
Response 201: { "filename": string, "size_bytes": number }
```

Idempotent. Re-uploading the same filename overwrites the stored file.

### Delete
```
DELETE /v2/media/:filename
Response 204
```

Decrements `ref_count`. If `ref_count` reaches zero the file is deleted from
object storage. Writes a tombstone.

---

## Batch endpoints

Single-item pushes are fine for conflict resolution, but initial sync of a
large collection needs batching.

### Batch push
```
POST /v2/batch/push
Body:
{
  "notes":     [{ "guid": string, ...note body }],
  "cards":     [{ "card_id": number, ...card body }],
  "reviews":   [{ "review_id": number, ...review-history fields }],
  "study_days":[{ "day": number, "deck_name": string, ...daily counters }],
  "notetypes": [{ "notetype_id": number, ...notetype body }],
  "decks":     [{ "name": string, ...deck body }]
}
Response 200:
{
  "accepted": { "notes": number, "cards": number, "reviews": number, "study_days": number, "notetypes": number, "decks": number },
  "conflicts": {
    "notes":     [{ "guid": string, "server": {...}, "client": {...} }],
    "reviews":   [{ "review_id": number, "server": {...}, "client": {...} }],
    "notetypes": [{ "notetype_id": number, "server": {...}, "client": {...} }],
    "decks":     [{ "name": string, "server": {...}, "client": {...} }]
  }
}
```

Cards never appear in conflicts (timestamp wins silently). Review rows conflict
only on an impossible/ambiguous reused id; notes, notetypes, and decks retain
their normal user-resolution behavior. The remaining rows are accepted.
`Force-Override: true` on a batch push accepts all items unconditionally.

### Batch pull
```
POST /v2/batch/pull
Body:
{
  "notes":     [string],    -- guids
  "cards":     [number],    -- card_ids
  "reviews":   [number],    -- review_ids
  "notetypes": [number],    -- notetype_ids
  "decks":     [string]     -- names
}
Response 200:
{
  "notes":     [...],
  "cards":     [...],
  "reviews":   [...],
  "notetypes": [...],
  "decks":     [...]
}
```

### Batch delete
```
POST /v2/batch/delete
Body:
{
  "notes":     [string],
  "cards":     [number],
  "notetypes": [number],
  "decks":     [string]
}
Response 200:
{
  "requested": { "notes": number, "cards": number, "notetypes": number, "decks": number },
  "deleted":   { "notes": number, "cards": number, "notetypes": number, "decks": number }
}
```

The delete and matching tombstone writes are transactional. A request is
limited to 12,000 resources; clients use 3,000-item batches and separately
require explicit approval for large deletion plans.

---

## Error shape

All error responses follow:
```json
{ "error": "short machine-readable string", "message": "human detail" }
```

Common status codes:
- `400` — malformed request body
- `401` — missing or invalid token
- `403` — token valid but action not permitted
- `404` — resource not found
- `409` — conflict (note / notetype / deck only)
- `413` — media file too large
- `429` — rate limited
- `500` — server error
