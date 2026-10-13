-- 0010_outbox_v1_status_idx — step 5 of 8.
-- ONE statement per file: CREATE/DROP INDEX CONCURRENTLY cannot run inside a
-- transaction, and golang-migrate sends a file with several statements as one
-- multi-statement query, which Postgres wraps in an implicit transaction. If it
-- fails it leaves an INVALID index behind: DROP INDEX it and run the migration again.

CREATE INDEX CONCURRENTLY IF NOT EXISTS identity_outbox_events_status_idx ON identity_outbox_events (status);
