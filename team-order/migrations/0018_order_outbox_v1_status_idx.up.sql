CREATE INDEX CONCURRENTLY IF NOT EXISTS order_outbox_events_status_idx ON order_outbox_events (status);
