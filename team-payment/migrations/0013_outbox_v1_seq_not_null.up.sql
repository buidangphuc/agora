ALTER TABLE payment_outbox_events ALTER COLUMN seq SET NOT NULL;
ALTER TABLE payment_outbox_events DROP CONSTRAINT payment_outbox_events_seq_nn;
