-- 0016_outbox_v1_pending_idx — step 5c of 10. One statement per file (CONCURRENTLY).
CREATE INDEX CONCURRENTLY IF NOT EXISTS payment_outbox_events_pending_idx ON payment_outbox_events (seq) WHERE status = 'pending';
