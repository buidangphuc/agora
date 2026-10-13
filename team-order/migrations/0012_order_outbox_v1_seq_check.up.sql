-- 0012_order_outbox_v1_seq_check -- outbox upgrade step 4a (first part of outbox.UpgradeSQL Finish).
--
-- NOT NULL without a long scan under ACCESS EXCLUSIVE: add a NOT VALID check
-- (instant), validate it in the next file (a weaker lock), then SET NOT NULL
-- reuses the validated check. The first statement lets a run that was
-- interrupted start over.
ALTER TABLE order_outbox_events DROP CONSTRAINT IF EXISTS order_outbox_events_seq_nn;
ALTER TABLE order_outbox_events ADD CONSTRAINT order_outbox_events_seq_nn CHECK (seq IS NOT NULL) NOT VALID;
