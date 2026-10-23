ALTER TABLE payment_outbox_events DROP CONSTRAINT IF EXISTS payment_outbox_events_seq_nn;
ALTER TABLE payment_outbox_events ADD CONSTRAINT payment_outbox_events_seq_nn CHECK (seq IS NOT NULL) NOT VALID;
