-- 0011_outbox_v1_agg_idx — step 6 of 8: the per-aggregate head scan.
-- ONE statement per file: CREATE/DROP INDEX CONCURRENTLY cannot run inside a
-- transaction, and golang-migrate sends a file with several statements as one
-- multi-statement query, which Postgres wraps in an implicit transaction. If it
-- fails it leaves an INVALID index behind: DROP INDEX it and run the migration again.

CREATE INDEX CONCURRENTLY IF NOT EXISTS identity_outbox_events_agg_idx ON identity_outbox_events (aggregate_type, aggregate_id, seq) WHERE status <> 'published';
