-- 0019: drop the v0 claim index, which the v1 relayer does not use. One statement,
-- because DROP INDEX CONCURRENTLY cannot run inside a transaction either.
DROP INDEX CONCURRENTLY IF EXISTS engagement_outbox_events_claim_idx;
