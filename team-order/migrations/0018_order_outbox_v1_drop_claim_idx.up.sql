-- 0018_order_outbox_v1_drop_claim_idx -- outbox upgrade step 5d, alone for
-- CONCURRENTLY (see 0015). idx_order_outbox_claim (created by 0006) served the v0
-- claim query; v1 claims through order_outbox_events_pending_idx. The aggregate
-- index of 0006 is left in place.
DROP INDEX CONCURRENTLY IF EXISTS idx_order_outbox_claim;
