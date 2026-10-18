DROP INDEX IF EXISTS orders_payment_due_at_pending_idx;
ALTER TABLE orders DROP COLUMN IF EXISTS cancel_reason;
ALTER TABLE orders DROP COLUMN IF EXISTS payment_reminder_sent_at;
ALTER TABLE orders DROP COLUMN IF EXISTS payment_due_at;
