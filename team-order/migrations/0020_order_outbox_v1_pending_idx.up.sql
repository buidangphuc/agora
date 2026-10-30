CREATE INDEX CONCURRENTLY IF NOT EXISTS order_outbox_events_pending_idx ON order_outbox_events (seq) WHERE status = 'pending';
