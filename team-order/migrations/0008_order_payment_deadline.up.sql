-- 0008_order_payment_deadline: unpaid-order expiry (change order-unpaid-expiry).
-- payment_due_at: deadline of an unpaid online-pay order (NULL for COD and for orders
-- placed before this migration; no backfill, those are never auto-expired).
-- payment_reminder_sent_at: stamped once when the payment reminder is emitted.
-- cancel_reason: closed enum, 0 = unspecified.
-- Additive and re-runnable.

ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS payment_due_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS payment_reminder_sent_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS cancel_reason INT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS orders_payment_due_at_pending_idx
    ON orders (payment_due_at)
    WHERE status = 1 AND payment_due_at IS NOT NULL;
