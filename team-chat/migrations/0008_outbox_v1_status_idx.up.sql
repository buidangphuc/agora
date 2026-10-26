CREATE INDEX CONCURRENTLY IF NOT EXISTS chat_outbox_events_status_idx ON chat_outbox_events (status);
