CREATE INDEX CONCURRENTLY IF NOT EXISTS identity_outbox_events_agg_idx ON identity_outbox_events (aggregate_type, aggregate_id, seq) WHERE status <> 'published';
