DROP INDEX IF EXISTS listings_seller_idx;
ALTER TABLE listings DROP COLUMN IF EXISTS seller_id;
