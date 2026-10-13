-- 0017 down — one statement per file (CONCURRENTLY): restore the v0 index.
CREATE INDEX CONCURRENTLY IF NOT EXISTS payment_outbox_events_claim_idx ON payment_outbox_events (available_at, created_at) WHERE status = 'pending';
