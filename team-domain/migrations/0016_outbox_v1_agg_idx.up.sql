-- One statement per file (see 0015). The v1 relayer finds each aggregate's
-- oldest unpublished row through this index.
CREATE INDEX CONCURRENTLY IF NOT EXISTS outbox_events_agg_idx
    ON outbox_events (aggregate_type, aggregate_id, seq) WHERE status <> 'published';
