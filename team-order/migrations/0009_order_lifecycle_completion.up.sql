-- 0009_order_lifecycle_completion: receipt confirmation, auto-complete, return window,
-- return auto-approve (change order-receipt-completion).
-- Backfill uses fixed values and completes nothing, emits nothing; it runs only when the
-- columns are first added, so the migration is re-runnable without moving a stamped deadline.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_schema = current_schema() AND table_name = 'orders' AND column_name = 'auto_complete_at') THEN
        ALTER TABLE orders
            ADD COLUMN shipped_at TIMESTAMPTZ,
            ADD COLUMN auto_complete_at TIMESTAMPTZ,
            ADD COLUMN completed_at TIMESTAMPTZ,
            ADD COLUMN return_window_ends_at TIMESTAMPTZ,
            ADD COLUMN ship_by TIMESTAMPTZ;
        ALTER TABLE order_returns ADD COLUMN auto_approve_at TIMESTAMPTZ;

        -- SHIPPED (3): shipped_at and auto_complete_at = migration time + 7 days.
        UPDATE orders SET shipped_at = now(), auto_complete_at = now() + interval '7 days'
         WHERE status = 3;
        -- COMPLETED (4): completed_at = updated_at, window 15 days later.
        UPDATE orders SET completed_at = updated_at, return_window_ends_at = updated_at + interval '15 days'
         WHERE status = 4;
        -- PAID (2) and COD PENDING (1, method 1): ship_by = (paid_at or created_at) + 2 days.
        UPDATE orders SET ship_by = COALESCE(paid_at, created_at) + interval '2 days'
         WHERE status = 2 OR (status = 1 AND payment_method = 1);
        -- PENDING returns (1): auto_approve_at = migration time + 3 days.
        UPDATE order_returns SET auto_approve_at = now() + interval '3 days'
         WHERE status = 1;
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS orders_auto_complete_at_shipped_idx
    ON orders (auto_complete_at)
    WHERE status = 3 AND auto_complete_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS order_returns_auto_approve_at_pending_idx
    ON order_returns (auto_approve_at)
    WHERE status = 1 AND auto_approve_at IS NOT NULL;
