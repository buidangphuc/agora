-- 0013_outbox_v1_seq_not_null — step 4c of 10: the validated check lets SET NOT NULL skip the scan.
ALTER TABLE payment_outbox_events ALTER COLUMN seq SET NOT NULL;
ALTER TABLE payment_outbox_events DROP CONSTRAINT payment_outbox_events_seq_nn;
