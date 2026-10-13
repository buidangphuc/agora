-- One statement per file (see 0015).
CREATE INDEX CONCURRENTLY IF NOT EXISTS outbox_events_pending_idx
    ON outbox_events (seq) WHERE status = 'pending';
