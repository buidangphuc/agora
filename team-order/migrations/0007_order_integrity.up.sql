-- 0007_order_integrity — checkout idempotency, payment time and a status backstop
-- (change port-order-inventory-correctness, design D8/D9/D13).
--
-- 1. order_sagas.idempotency_key: the client's Idempotency-Key, unique per buyer, so
--    a retried checkout replays the original attempt's orders. Compensation sets it
--    back to NULL, which frees the key for a fresh checkout.
-- 2. orders.paid_at: the moment the Pending -> Paid compare-and-set succeeded. NULL
--    for orders paid before this migration ("paid, time not recorded").
-- 3. orders_status_check: no write may store a status outside Pending(1)..Cancelled(5).
--    Added NOT VALID (no scan, no long lock); validate later, after
--      SELECT count(*) FROM orders WHERE status NOT BETWEEN 1 AND 5;
--    returns 0, with
--      ALTER TABLE orders VALIDATE CONSTRAINT orders_status_check;
--
-- Additive and re-runnable (the Postgres test harness re-applies every *.up.sql).

ALTER TABLE order_sagas
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS order_sagas_buyer_idem_key_uidx
    ON order_sagas (buyer_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS paid_at TIMESTAMPTZ;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'orders_status_check'
          AND conrelid = 'orders'::regclass
    ) THEN
        ALTER TABLE orders
            ADD CONSTRAINT orders_status_check CHECK (status BETWEEN 1 AND 5) NOT VALID;
    END IF;
END
$$;
