-- 0005_variants — Product variants and inventory management (Phase 3).

-- Add base stock to listings table
ALTER TABLE listings ADD COLUMN IF NOT EXISTS stock INT NOT NULL DEFAULT 0;

-- Create listing_variants table
CREATE TABLE IF NOT EXISTS listing_variants (
    id          TEXT PRIMARY KEY,
    listing_id  TEXT NOT NULL REFERENCES listings(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    sku         TEXT NOT NULL DEFAULT '',
    price       BIGINT NOT NULL DEFAULT 0,
    stock       INT NOT NULL DEFAULT 0,
    image_url   TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS listing_variants_listing_idx ON listing_variants (listing_id);
