-- 004_review_history.sql
--
-- KelmaSync v2 review history is append-only, matching Anki's native sync
-- semantics.  Review rows use Anki's millisecond review id as their portable
-- identity, while note_guid + card_ord let clients remap collection-local card
-- ids on download.

CREATE TABLE reviews (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    review_id       BIGINT      NOT NULL,
    source_card_id  BIGINT      NOT NULL DEFAULT 0,
    note_guid       TEXT        NOT NULL DEFAULT '',
    card_ord        INT         NOT NULL DEFAULT 0,
    deck_name       TEXT        NOT NULL DEFAULT '',
    ease            SMALLINT    NOT NULL,
    interval        INT         NOT NULL,
    last_interval   INT         NOT NULL,
    factor          INT         NOT NULL,
    taken_millis    INT         NOT NULL,
    review_kind     SMALLINT    NOT NULL,
    checksum        TEXT        NOT NULL,
    modified_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_client_id  UUID        REFERENCES clients(id),
    UNIQUE (user_id, review_id)
);

CREATE INDEX idx_reviews_user_modified
    ON reviews (user_id, modified_at);
CREATE INDEX idx_reviews_user_card
    ON reviews (user_id, note_guid, card_ord, review_id);

-- Anki's per-day limits are not derived from card scheduling.  They live in
-- collection-relative deck counters, so clients publish a portable epoch-day
-- snapshot alongside review history.  Same-day snapshots merge monotonically;
-- this mirrors review history's append-only behavior and prevents an older
-- device from reopening a quota already consumed elsewhere.
CREATE TABLE study_days (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id               TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day                   BIGINT      NOT NULL,
    deck_name             TEXT        NOT NULL,
    new_studied           INT         NOT NULL DEFAULT 0,
    review_studied        INT         NOT NULL DEFAULT 0,
    learning_studied      INT         NOT NULL DEFAULT 0,
    milliseconds_studied  BIGINT      NOT NULL DEFAULT 0,
    modified_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_client_id        UUID        REFERENCES clients(id),
    UNIQUE (user_id, day, deck_name)
);

CREATE INDEX idx_study_days_user_modified
    ON study_days (user_id, modified_at);
CREATE INDEX idx_study_days_user_day
    ON study_days (user_id, day);
