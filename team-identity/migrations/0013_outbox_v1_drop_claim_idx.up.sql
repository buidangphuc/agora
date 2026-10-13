-- 0013_outbox_v1_drop_claim_idx — step 8 of 8: the v0 claim index is unused by v1.
-- ONE statement per file: CREATE/DROP INDEX CONCURRENTLY cannot run inside a
-- transaction, and golang-migrate sends a file with several statements as one
-- multi-statement query, which Postgres wraps in an implicit transaction. If it
-- fails it leaves an INVALID index behind: DROP INDEX it and run the migration again.

DROP INDEX CONCURRENTLY IF EXISTS identity_outbox_claim_idx;
