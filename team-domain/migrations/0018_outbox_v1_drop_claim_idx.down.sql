CREATE INDEX CONCURRENTLY IF NOT EXISTS outbox_events_claim_idx
    ON outbox_events (available_at, created_at)
    WHERE status = 'pending';
