DROP TABLE IF EXISTS new_user_voucher_state;
DROP TABLE IF EXISTS promotion_processed_events;

DROP INDEX IF EXISTS voucher_reservations_buyer_idx;
ALTER TABLE voucher_reservations DROP COLUMN IF EXISTS expires_at;

DROP INDEX IF EXISTS vouchers_owner_idx;
DROP INDEX IF EXISTS vouchers_new_user_owner_unique;
ALTER TABLE vouchers DROP CONSTRAINT IF EXISTS vouchers_new_user_owner_check;
ALTER TABLE vouchers DROP COLUMN IF EXISTS new_user_only;
ALTER TABLE vouchers DROP COLUMN IF EXISTS per_user_limit;
ALTER TABLE vouchers DROP COLUMN IF EXISTS kind;
ALTER TABLE vouchers DROP COLUMN IF EXISTS owner_user_id;
