DELETE FROM cart_items WHERE bundle_id <> '';
DROP INDEX IF EXISTS cart_items_unique_user_item_bundle;
ALTER TABLE cart_items ADD CONSTRAINT cart_items_unique_user_item UNIQUE (user_id, listing_id, variant_id);
ALTER TABLE cart_items DROP COLUMN IF EXISTS bundle_discount;
ALTER TABLE cart_items DROP COLUMN IF EXISTS bundle_id;
ALTER TABLE order_items DROP COLUMN IF EXISTS bundle_discount;
ALTER TABLE order_items DROP COLUMN IF EXISTS bundle_id;
