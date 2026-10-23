-- Durable consumer state, so a restart or redeploy neither re-notifies a redelivered
-- event nor loses the baseline needed to detect a price drop / restock.
-- Owned by team-notification.

-- Idempotency ledger (ADR-0002 / AD4): one row per (consumer, event_id) already
-- applied. The consumer name namespaces the listing, chat and order consumers.
CREATE TABLE IF NOT EXISTS processed_events (
    consumer     TEXT        NOT NULL,
    event_id     TEXT        NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (consumer, event_id)
);

-- Last-seen listing price/stock: the "old" side of the self-diff over
-- ListingChanged snapshots. A NULL column means "unknown prior" (a first-ever
-- snapshot never fires), so price and stock are tracked independently.
CREATE TABLE IF NOT EXISTS listing_last_seen (
    listing_id TEXT        PRIMARY KEY,
    price      BIGINT,
    stock      INT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
