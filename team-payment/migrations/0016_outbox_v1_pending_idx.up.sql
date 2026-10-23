CREATE INDEX CONCURRENTLY IF NOT EXISTS payment_outbox_events_pending_idx ON payment_outbox_events (seq) WHERE status = 'pending';
