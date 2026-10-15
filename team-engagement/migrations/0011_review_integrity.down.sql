DROP INDEX IF EXISTS uq_reviews_order_listing_verified;
ALTER TABLE reviews DROP COLUMN IF EXISTS hide_reason;
ALTER TABLE reviews DROP COLUMN IF EXISTS hidden_by;
ALTER TABLE reviews DROP COLUMN IF EXISTS hidden_at;
