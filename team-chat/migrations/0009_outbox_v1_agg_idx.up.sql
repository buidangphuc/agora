CREATE INDEX CONCURRENTLY IF NOT EXISTS chat_outbox_events_agg_idx ON chat_outbox_events (aggregate_type, aggregate_id, seq) WHERE status <> 'published';
