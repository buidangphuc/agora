-- 0002_shipping_and_payment — Add shipping_fee, items_subtotal, and payment_method to orders (Phase 5).

ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS shipping_fee   BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS items_subtotal BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS payment_method INT NOT NULL DEFAULT 1;
