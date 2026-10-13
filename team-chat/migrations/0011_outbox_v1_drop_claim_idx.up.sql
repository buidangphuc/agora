-- go-sdk v1 outbox upgrade, step 8 of 8. Drops the v0 claim index, which v1 does not
-- use. outbox.UpgradeSQL names it <table>_claim_idx; migration 0003 created it as
-- idx_chat_outbox_claim, so that is the one dropped here. One CONCURRENTLY
-- statement per file.
DROP INDEX CONCURRENTLY IF EXISTS idx_chat_outbox_claim;
