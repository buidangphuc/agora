-- go-sdk v1 outbox upgrade, step 7 of 8. One CONCURRENTLY statement per file; if it
-- fails, DROP the invalid index and rerun.
CREATE INDEX CONCURRENTLY IF NOT EXISTS chat_outbox_events_pending_idx ON chat_outbox_events (seq) WHERE status = 'pending';
