CREATE INDEX CONCURRENTLY IF NOT EXISTS payment_outbox_events_status_idx ON payment_outbox_events (status);
