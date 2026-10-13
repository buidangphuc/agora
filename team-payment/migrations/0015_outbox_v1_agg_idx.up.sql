-- 0015_outbox_v1_agg_idx — step 5b of 10. One statement per file (CONCURRENTLY).
CREATE INDEX CONCURRENTLY IF NOT EXISTS payment_outbox_events_agg_idx ON payment_outbox_events (aggregate_type, aggregate_id, seq) WHERE status <> 'published';
