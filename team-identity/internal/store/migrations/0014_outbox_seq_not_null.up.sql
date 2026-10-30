ALTER TABLE identity_outbox_events ALTER COLUMN seq SET NOT NULL;
ALTER TABLE identity_outbox_events DROP CONSTRAINT identity_outbox_events_seq_nn;
