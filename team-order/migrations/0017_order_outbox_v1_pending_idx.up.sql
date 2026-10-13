-- 0017_order_outbox_v1_pending_idx -- outbox upgrade step 5c, alone for CONCURRENTLY
-- (see 0015). Serves the claim of pending rows in seq order.
CREATE INDEX CONCURRENTLY IF NOT EXISTS order_outbox_events_pending_idx ON order_outbox_events (seq) WHERE status = 'pending';
