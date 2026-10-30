ALTER TABLE order_outbox_events ALTER COLUMN seq SET NOT NULL;
ALTER TABLE order_outbox_events DROP CONSTRAINT order_outbox_events_seq_nn;
