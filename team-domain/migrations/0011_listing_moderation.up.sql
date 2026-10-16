-- 0011_listing_moderation: moderation state on listings (OpenSpec change
-- domain-listing-moderation). status gains pending_review | held | banned (it is
-- free TEXT, no enum to alter). All columns are additive with defaults.
ALTER TABLE listings
    ADD COLUMN IF NOT EXISTS moderation_reason_code  TEXT        NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS moderation_note         TEXT        NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS screen_flags            TEXT[]      NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS moderation_decided_at   TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS submitted_at            TIMESTAMPTZ;

-- Moderation queue: oldest submission first, keyset on (submitted_at, id).
CREATE INDEX IF NOT EXISTS listings_moderation_queue_idx
    ON listings (submitted_at, id)
    WHERE status = 'pending_review';
