-- 0011_outbox_v1_seq_check — step 4a of 10: NOT NULL without a long scan under ACCESS EXCLUSIVE.
--
-- Adds the check NOT VALID (instant). 0012 validates it in its own transaction so the scan does
-- not hold the lock taken here; the first statement makes a rerun after an interruption safe.
ALTER TABLE payment_outbox_events DROP CONSTRAINT IF EXISTS payment_outbox_events_seq_nn;
ALTER TABLE payment_outbox_events ADD CONSTRAINT payment_outbox_events_seq_nn CHECK (seq IS NOT NULL) NOT VALID;
