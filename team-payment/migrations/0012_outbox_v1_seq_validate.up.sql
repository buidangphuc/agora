-- 0012_outbox_v1_seq_validate — step 4b of 10: validate the check (SHARE UPDATE EXCLUSIVE only).
ALTER TABLE payment_outbox_events VALIDATE CONSTRAINT payment_outbox_events_seq_nn;
