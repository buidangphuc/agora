DROP INDEX IF EXISTS orders_checkout_id_idx;
ALTER TABLE orders DROP COLUMN IF EXISTS platform_discount_amount;
ALTER TABLE orders DROP COLUMN IF EXISTS platform_voucher_code;
ALTER TABLE orders DROP COLUMN IF EXISTS checkout_id;
