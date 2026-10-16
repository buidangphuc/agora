DROP INDEX IF EXISTS listings_seller_hold_idx;
DROP TABLE IF EXISTS seller_status_applied;
DROP TABLE IF EXISTS processed_events;
ALTER TABLE listings DROP COLUMN IF EXISTS held_prior_status;
