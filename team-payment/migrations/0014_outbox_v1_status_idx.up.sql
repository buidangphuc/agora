-- 0014_outbox_v1_status_idx — step 5a of 10. ONE statement per file: CREATE INDEX CONCURRENTLY
-- cannot run in a transaction, and the migrate runner sends a file as one simple query (several
-- statements in it would form an implicit transaction). If it fails, DROP the invalid index and
-- rerun the migration (IF NOT EXISTS; see the README).
CREATE INDEX CONCURRENTLY IF NOT EXISTS payment_outbox_events_status_idx ON payment_outbox_events (status);
