-- 0005_voucher_redemption down — rollback voucher columns.
ALTER TABLE orders
    DROP COLUMN IF EXISTS discount_amount,
    DROP COLUMN IF EXISTS voucher_code;
