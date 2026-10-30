CREATE INDEX CONCURRENTLY IF NOT EXISTS identity_outbox_events_pending_idx ON identity_outbox_events (seq) WHERE status = 'pending';
