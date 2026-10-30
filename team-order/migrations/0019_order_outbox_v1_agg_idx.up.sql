CREATE INDEX CONCURRENTLY IF NOT EXISTS order_outbox_events_agg_idx ON order_outbox_events (aggregate_type, aggregate_id, seq) WHERE status <> 'published';
