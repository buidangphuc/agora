CREATE INDEX CONCURRENTLY IF NOT EXISTS chat_outbox_events_pending_idx ON chat_outbox_events (seq) WHERE status = 'pending';
