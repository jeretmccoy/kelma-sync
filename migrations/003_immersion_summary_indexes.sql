-- Purpose-built Kelma Immersion summary reads filter/group cards by deck name,
-- and the one-request card creator reuses its shared notetype by name.
CREATE INDEX IF NOT EXISTS idx_cards_user_deck
    ON cards (user_id, deck_name);

CREATE INDEX IF NOT EXISTS idx_notetypes_user_name
    ON notetypes (user_id, name);
