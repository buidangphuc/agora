-- 0004_flash_sale_holds — per-reservation stock holds for flash-sale campaigns.
-- Reserve inserts a 'reserved' row (expires 15 minutes later) under a FOR UPDATE lock
-- on the campaign row; Commit adds the quantity to flash_sale_campaigns.stock_sold;
-- Release returns the units. UNIQUE(reservation_id) enforces idempotency.

CREATE TABLE IF NOT EXISTS flash_sale_holds (
    reservation_id TEXT PRIMARY KEY,
    campaign_id    TEXT NOT NULL REFERENCES flash_sale_campaigns (id),
    listing_id     TEXT NOT NULL,
    variant_id     TEXT NOT NULL DEFAULT '',
    quantity       BIGINT NOT NULL CHECK (quantity > 0),
    status         TEXT NOT NULL DEFAULT 'reserved',  -- reserved | committed | released
    expires_at     TIMESTAMPTZ NOT NULL,
    released_at    TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT flash_sale_holds_reservation_unique UNIQUE (reservation_id)
);

CREATE INDEX IF NOT EXISTS flash_sale_holds_campaign_idx ON flash_sale_holds (campaign_id, status);
