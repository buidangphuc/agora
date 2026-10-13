-- 0017_outbox_v1_drop_claim_idx — step 5d of 10: the v0 claim index is unused by v1.
-- One statement per file (CONCURRENTLY).
DROP INDEX CONCURRENTLY IF EXISTS payment_outbox_events_claim_idx;
