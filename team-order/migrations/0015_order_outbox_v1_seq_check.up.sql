ALTER TABLE order_outbox_events DROP CONSTRAINT IF EXISTS order_outbox_events_seq_nn;
ALTER TABLE order_outbox_events ADD CONSTRAINT order_outbox_events_seq_nn CHECK (seq IS NOT NULL) NOT VALID;
