-- Owner column for the seller center (ListMyListings + edit-own authz).
ALTER TABLE listings ADD COLUMN IF NOT EXISTS seller_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS listings_seller_idx ON listings (seller_id);
