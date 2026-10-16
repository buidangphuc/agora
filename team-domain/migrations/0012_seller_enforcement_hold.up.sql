-- 0012_seller_enforcement_hold: seller suspension/ban holds (OpenSpec change
-- domain-seller-enforcement-hold). held_prior_status remembers the status a
-- listing had before a seller hold; processed_events dedupes redelivered
-- UserStatusChanged events; seller_status_applied guards against stale events.
ALTER TABLE listings
    ADD COLUMN IF NOT EXISTS held_prior_status TEXT;

CREATE TABLE IF NOT EXISTS processed_events (
    event_id     TEXT        PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS seller_status_applied (
    user_id    TEXT        PRIMARY KEY,
    changed_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS listings_seller_hold_idx
    ON listings (seller_id)
    WHERE status IN ('published', 'pending_review', 'held');
