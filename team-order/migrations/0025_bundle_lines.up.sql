ALTER TABLE cart_items ADD COLUMN IF NOT EXISTS bundle_id TEXT NOT NULL DEFAULT '';
ALTER TABLE cart_items ADD COLUMN IF NOT EXISTS bundle_discount BIGINT NOT NULL DEFAULT 0;
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS bundle_id TEXT NOT NULL DEFAULT '';
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS bundle_discount BIGINT NOT NULL DEFAULT 0;
ALTER TABLE cart_items DROP CONSTRAINT IF EXISTS cart_items_unique_user_item;
CREATE UNIQUE INDEX IF NOT EXISTS cart_items_unique_user_item_bundle ON cart_items (user_id, listing_id, variant_id, bundle_id);
