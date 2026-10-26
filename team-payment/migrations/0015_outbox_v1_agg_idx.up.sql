CREATE INDEX CONCURRENTLY IF NOT EXISTS payment_outbox_events_agg_idx ON payment_outbox_events (aggregate_type, aggregate_id, seq) WHERE status <> 'published';
