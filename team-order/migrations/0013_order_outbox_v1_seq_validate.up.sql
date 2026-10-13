-- 0013_order_outbox_v1_seq_validate -- outbox upgrade step 4b: its own file so the
-- scan runs under SHARE UPDATE EXCLUSIVE, not together with the ACCESS EXCLUSIVE
-- statements of the neighbouring files.
ALTER TABLE order_outbox_events VALIDATE CONSTRAINT order_outbox_events_seq_nn;
