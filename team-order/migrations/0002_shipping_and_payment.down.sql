-- 0002_shipping_and_payment down — rollback columns.
ALTER TABLE orders
    DROP COLUMN IF EXISTS payment_method,
    DROP COLUMN IF EXISTS items_subtotal,
    DROP COLUMN IF EXISTS shipping_fee;
