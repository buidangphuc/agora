-- Durable per-listing low-stock state: owner and title (from ListingChanged
-- snapshots) plus whether the listing is currently at/below the threshold, so a
-- restart neither repeats nor loses a low-stock notification.
CREATE TABLE IF NOT EXISTS listing_low_stock_state (
    listing_id TEXT        PRIMARY KEY,
    seller_id  TEXT        NOT NULL,
    title      TEXT        NOT NULL DEFAULT '',
    low        BOOLEAN     NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
