-- 0018: v1 outbox index. CREATE INDEX CONCURRENTLY cannot run inside a transaction,
-- so this file holds this one statement and nothing else (golang-migrate sends a
-- file as one query, and a single statement is not wrapped in a transaction). If it
-- fails, DROP the invalid index it left and apply the migration again.
CREATE INDEX CONCURRENTLY IF NOT EXISTS engagement_outbox_events_pending_idx ON engagement_outbox_events (seq) WHERE status = 'pending';
