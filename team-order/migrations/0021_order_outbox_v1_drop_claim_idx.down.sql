CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_order_outbox_claim ON order_outbox_events (available_at, created_at) WHERE status = 'pending';
