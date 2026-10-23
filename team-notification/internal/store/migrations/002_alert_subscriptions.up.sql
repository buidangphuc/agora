-- Alert subscriptions: a user subscribes to price-drop / back-in-stock alerts
-- for a listing. Owned by team-notification (engagement stays collections-only).
CREATE TABLE IF NOT EXISTS alert_subscriptions (
    id         VARCHAR(64) PRIMARY KEY,
    user_id    VARCHAR(64) NOT NULL,
    listing_id VARCHAR(64) NOT NULL,
    type       INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One subscription per (user, listing, type): makes Subscribe idempotent and
-- lets the listing.events consumer fan out to distinct subscribers.
CREATE UNIQUE INDEX IF NOT EXISTS uq_alert_sub_user_listing_type
    ON alert_subscriptions (user_id, listing_id, type);

-- The consumer looks subscriptions up by (listing_id, type) to find who to notify.
CREATE INDEX IF NOT EXISTS idx_alert_sub_listing_type
    ON alert_subscriptions (listing_id, type);
