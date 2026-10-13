-- One statement per file (see 0015). The v1 relayer does not use claim_idx.
DROP INDEX CONCURRENTLY IF EXISTS outbox_events_claim_idx;
