-- 0016_order_outbox_v1_agg_idx -- outbox upgrade step 5b, alone for CONCURRENTLY
-- (see 0015). Serves the per-aggregate order scan of the v1 relayer.
CREATE INDEX CONCURRENTLY IF NOT EXISTS order_outbox_events_agg_idx ON order_outbox_events (aggregate_type, aggregate_id, seq) WHERE status <> 'published';
