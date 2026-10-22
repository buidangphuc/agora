ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS checkout_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS platform_voucher_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS platform_discount_amount BIGINT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS orders_checkout_id_idx ON orders (checkout_id) WHERE checkout_id <> '';
