-- 0014_order_outbox_v1_seq_not_null -- outbox upgrade step 4c: the validated check
-- lets SET NOT NULL skip the table scan; the helper constraint is then redundant.
ALTER TABLE order_outbox_events ALTER COLUMN seq SET NOT NULL;
ALTER TABLE order_outbox_events DROP CONSTRAINT order_outbox_events_seq_nn;
