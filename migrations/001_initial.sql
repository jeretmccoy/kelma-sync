-- 001_initial.sql

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ------------------------------------------------------------------ users ---

CREATE TABLE users (
    -- Stable account id derived from the lowercase email, matching v1 gateway:
    -- sha256(email)[0:32]. This lets v2 authenticate against the existing prod
    -- Mongo/Immersion accounts without migrating user ids.
    id            TEXT        PRIMARY KEY,
    username      TEXT        NOT NULL UNIQUE,
    password_hash TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------- clients ---

CREATE TABLE clients (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label      TEXT        NOT NULL,
    last_seen  TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, label)
);

-- ----------------------------------------------------------------- tokens ---

CREATE TABLE tokens (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id  UUID        NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    token_hash TEXT        NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- --------------------------------------------------------------- notetypes ---

CREATE TABLE notetypes (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notetype_id        BIGINT      NOT NULL,
    name               TEXT        NOT NULL,
    definition         JSONB       NOT NULL,
    checksum           TEXT        NOT NULL,
    modified_at        TIMESTAMPTZ NOT NULL,
    client_modified_at TIMESTAMPTZ NOT NULL,
    last_client_id     UUID        REFERENCES clients(id),
    UNIQUE (user_id, notetype_id)
);

-- ------------------------------------------------------------------ decks ---

CREATE TABLE decks (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name               TEXT        NOT NULL,
    config             JSONB       NOT NULL DEFAULT '{}',
    checksum           TEXT        NOT NULL,
    modified_at        TIMESTAMPTZ NOT NULL,
    client_modified_at TIMESTAMPTZ NOT NULL,
    last_client_id     UUID        REFERENCES clients(id),
    UNIQUE (user_id, name)
);

-- ------------------------------------------------------------------ notes ---

CREATE TABLE notes (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    guid               TEXT        NOT NULL,
    notetype_id        BIGINT      NOT NULL,
    fields             JSONB       NOT NULL,
    tags               TEXT[]      NOT NULL DEFAULT '{}',
    checksum           TEXT        NOT NULL,
    modified_at        TIMESTAMPTZ NOT NULL,
    client_modified_at TIMESTAMPTZ NOT NULL,
    last_client_id     UUID        REFERENCES clients(id),
    UNIQUE (user_id, guid)
);

-- ------------------------------------------------------------------ cards ---

CREATE TABLE cards (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    card_id            BIGINT      NOT NULL,
    note_guid          TEXT        NOT NULL,
    deck_name          TEXT        NOT NULL,
    ord                INT         NOT NULL,
    scheduling         JSONB       NOT NULL DEFAULT '{}',
    modified_at        TIMESTAMPTZ NOT NULL,
    client_modified_at TIMESTAMPTZ NOT NULL,
    last_client_id     UUID        REFERENCES clients(id),
    UNIQUE (user_id, card_id)
);

-- ------------------------------------------------------------------ media ---

CREATE TABLE media (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    filename    TEXT        NOT NULL,
    size_bytes  BIGINT      NOT NULL,
    storage_key TEXT,
    ref_count   INT         NOT NULL DEFAULT 0,
    modified_at TIMESTAMPTZ NOT NULL,
    UNIQUE (user_id, filename)
);

-- ------------------------------------------------------------- tombstones ---

CREATE TABLE tombstones (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type           TEXT        NOT NULL CHECK (type IN ('note','card','deck','notetype','media')),
    resource_id    TEXT        NOT NULL,
    deleted_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_client_id UUID        REFERENCES clients(id),
    UNIQUE (user_id, type, resource_id)
);

-- ---------------------------------------------------------------- indexes ---

CREATE INDEX idx_notetypes_user_modified  ON notetypes  (user_id, modified_at);
CREATE INDEX idx_decks_user_modified      ON decks      (user_id, modified_at);
CREATE INDEX idx_notes_user_modified      ON notes      (user_id, modified_at);
CREATE INDEX idx_notes_guid               ON notes      (user_id, guid);
CREATE INDEX idx_cards_user_modified      ON cards      (user_id, modified_at);
CREATE INDEX idx_cards_card_id            ON cards      (user_id, card_id);
CREATE INDEX idx_cards_note_guid          ON cards      (user_id, note_guid);
CREATE INDEX idx_media_user_modified      ON media      (user_id, modified_at);
CREATE INDEX idx_tombstones_user_deleted  ON tombstones (user_id, deleted_at);
