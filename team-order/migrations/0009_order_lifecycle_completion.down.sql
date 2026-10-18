DROP INDEX IF EXISTS order_returns_auto_approve_at_pending_idx;
DROP INDEX IF EXISTS orders_auto_complete_at_shipped_idx;
ALTER TABLE order_returns DROP COLUMN IF EXISTS auto_approve_at;
ALTER TABLE orders DROP COLUMN IF EXISTS ship_by;
ALTER TABLE orders DROP COLUMN IF EXISTS return_window_ends_at;
ALTER TABLE orders DROP COLUMN IF EXISTS completed_at;
ALTER TABLE orders DROP COLUMN IF EXISTS auto_complete_at;
ALTER TABLE orders DROP COLUMN IF EXISTS shipped_at;
