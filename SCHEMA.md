# KelmaSync v2 — Database Schema

Postgres. One database per deployment, one schema per user (or row-level
security with a `user_id` column — TBD based on scale). Examples below use
a single shared schema with `user_id` on every table.

---

## Users and clients

```sql
CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username    TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE clients (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label       TEXT NOT NULL,  -- "iPhone 15", "MacBook — Anki plugin"
    last_seen   TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id   UUID NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

---

## Notetypes

```sql
CREATE TABLE notetypes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notetype_id     BIGINT NOT NULL,   -- Anki's local id, for client mapping
    name            TEXT NOT NULL,
    -- full definition as JSON: fields, templates, css, config
    definition      JSONB NOT NULL,
    checksum        TEXT NOT NULL,     -- hash of definition
    modified_at     TIMESTAMPTZ NOT NULL,
    client_modified_at TIMESTAMPTZ NOT NULL,
    last_client_id  UUID REFERENCES clients(id),
    UNIQUE (user_id, notetype_id)
);
```

---

## Decks

```sql
CREATE TABLE decks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,   -- full path, e.g. "Japanese::Vocab"
    -- scheduler config: new/day, review limits, etc. as JSON
    config      JSONB NOT NULL DEFAULT '{}',
    checksum    TEXT NOT NULL,   -- hash of config
    modified_at TIMESTAMPTZ NOT NULL,
    client_modified_at TIMESTAMPTZ NOT NULL,
    last_client_id UUID REFERENCES clients(id),
    UNIQUE (user_id, name)
);
```

---

## Notes

```sql
CREATE TABLE notes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    guid        TEXT NOT NULL,
    notetype_id BIGINT NOT NULL,   -- references notetypes.notetype_id
    -- fields stored as ordered JSON array, one entry per notetype field
    fields      JSONB NOT NULL,
    tags        TEXT[] NOT NULL DEFAULT '{}',
    checksum    TEXT NOT NULL,     -- hash of fields + tags
    modified_at TIMESTAMPTZ NOT NULL,
    client_modified_at TIMESTAMPTZ NOT NULL,
    last_client_id UUID REFERENCES clients(id),
    UNIQUE (user_id, guid)
);
```

---

## Cards

```sql
CREATE TABLE cards (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    card_id     BIGINT NOT NULL,   -- Anki's local card id, for client mapping
    note_guid   TEXT NOT NULL,     -- references notes.guid
    deck_name   TEXT NOT NULL,     -- references decks.name
    ord         INT NOT NULL,      -- template index within the notetype
    -- scheduling state as JSON: due, interval, factor, reps, lapses, type, queue
    scheduling  JSONB NOT NULL DEFAULT '{}',
    modified_at TIMESTAMPTZ NOT NULL,
    client_modified_at TIMESTAMPTZ NOT NULL,
    last_client_id UUID REFERENCES clients(id),
    UNIQUE (user_id, card_id)
);
```

---

## Review history

```sql
CREATE TABLE reviews (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    review_id       BIGINT NOT NULL,
    source_card_id  BIGINT NOT NULL DEFAULT 0,
    note_guid       TEXT NOT NULL DEFAULT '',
    card_ord        INT NOT NULL DEFAULT 0,
    deck_name       TEXT NOT NULL DEFAULT '',
    ease            SMALLINT NOT NULL,
    interval        INT NOT NULL,
    last_interval   INT NOT NULL,
    factor          INT NOT NULL,
    taken_millis    INT NOT NULL,
    review_kind     SMALLINT NOT NULL,
    checksum        TEXT NOT NULL,
    modified_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_client_id  UUID REFERENCES clients(id),
    UNIQUE (user_id, review_id)
);
```

Review rows are append-only, like native Anki sync. `source_card_id` is retained
as a fallback, but clients map history through `(note_guid, card_ord)` because
card ids differ between collections.

## Daily study counters

```sql
CREATE TABLE study_days (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id               TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day                   BIGINT NOT NULL,
    deck_name             TEXT NOT NULL,
    new_studied           INT NOT NULL DEFAULT 0,
    review_studied        INT NOT NULL DEFAULT 0,
    learning_studied      INT NOT NULL DEFAULT 0,
    milliseconds_studied  BIGINT NOT NULL DEFAULT 0,
    modified_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_client_id        UUID REFERENCES clients(id),
    UNIQUE (user_id, day, deck_name)
);
```

`day` is a portable epoch-day. Clients translate between it and Anki's
collection-relative deck day using the collection creation day (`crt`).

---

## Media

```sql
CREATE TABLE media (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    filename    TEXT NOT NULL,
    size_bytes  BIGINT NOT NULL,
    -- storage key in object store (S3/R2); NULL if not yet uploaded
    storage_key TEXT,
    -- reference count: number of notes whose fields mention this filename
    ref_count   INT NOT NULL DEFAULT 0,
    modified_at TIMESTAMPTZ NOT NULL,
    UNIQUE (user_id, filename)
);
```

---

## Tombstones

```sql
CREATE TABLE tombstones (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- which resource type was deleted
    type        TEXT NOT NULL CHECK (type IN ('note', 'card', 'deck', 'notetype', 'media')),
    -- the stable identifier for the deleted resource
    -- (guid for notes, card_id for cards, name for decks, notetype_id for notetypes, filename for media)
    resource_id TEXT NOT NULL,
    deleted_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_client_id UUID REFERENCES clients(id),
    UNIQUE (user_id, type, resource_id)
);
```

---

## Indexes

```sql
-- manifest queries: "give me everything changed since <timestamp>"
CREATE INDEX idx_notes_user_modified      ON notes      (user_id, modified_at);
CREATE INDEX idx_cards_user_modified      ON cards      (user_id, modified_at);
CREATE INDEX idx_reviews_user_modified    ON reviews    (user_id, modified_at);
CREATE INDEX idx_study_days_user_modified ON study_days (user_id, modified_at);
CREATE INDEX idx_notetypes_user_modified  ON notetypes  (user_id, modified_at);
CREATE INDEX idx_decks_user_modified      ON decks      (user_id, modified_at);
CREATE INDEX idx_tombstones_user_modified ON tombstones (user_id, deleted_at);

-- individual record lookups
CREATE INDEX idx_notes_guid       ON notes  (user_id, guid);
CREATE INDEX idx_cards_card_id    ON cards  (user_id, card_id);
CREATE INDEX idx_cards_note_guid  ON cards  (user_id, note_guid);
CREATE INDEX idx_media_filename   ON media  (user_id, filename);
```

---

## Notes on design choices

- **JSONB for fields and scheduling** — note fields are an ordered array whose
  length and meaning are defined by the notetype. Scheduling is a bag of
  Anki-specific integers that may grow over time. Both are opaque to the server
  beyond storage and retrieval; JSONB avoids a schema migration every time Anki
  adds a scheduler field.
- **`modified_at` vs `client_modified_at`** — `modified_at` is the server
  timestamp of the last accepted write (used for manifest filtering).
  `client_modified_at` is what the client reported; used for per-card scheduling
  conflict resolution (newest client timestamp wins) and displayed in the
  conflict UI.
- **`notetype_id` as BIGINT not UUID** — Anki assigns notetype ids as
  millisecond timestamps locally. Storing the original id avoids a mapping layer
  on the client.
- **`deck_name` on cards, not a foreign key** — decks are identified by name.
  Storing the name directly on each card avoids a join and keeps card records
  self-contained for sync.
- **Append-only reviews** — Anki does not synchronize revlog deletion/undo.
  Review ids are immutable millisecond ids; an id reused with different content
  is surfaced as a conflict instead of silently dropping either history row.
- **Portable daily counters** — revlog rows power statistics, while Anki's daily
  limits use separate deck counters. Epoch-day snapshots keep those limits in
  sync even when collections have different creation timestamps.
- **Media `ref_count`** — incremented when a note is pushed whose fields
  reference the filename, decremented on note update/delete. When `ref_count`
  reaches zero the storage object can be deleted. Not enforced as a DB
  constraint; maintained by application logic.
