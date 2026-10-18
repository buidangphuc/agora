-- 0011_listing_moderation (down)
DROP INDEX IF EXISTS listings_moderation_queue_idx;
ALTER TABLE listings
    DROP COLUMN IF EXISTS submitted_at,
    DROP COLUMN IF EXISTS moderation_decided_at,
    DROP COLUMN IF EXISTS screen_flags,
    DROP COLUMN IF EXISTS moderation_note,
    DROP COLUMN IF EXISTS moderation_reason_code;
