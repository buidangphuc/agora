ALTER TABLE identity_outbox_events DROP CONSTRAINT IF EXISTS identity_outbox_events_seq_nn;
ALTER TABLE identity_outbox_events ADD CONSTRAINT identity_outbox_events_seq_nn CHECK (seq IS NOT NULL) NOT VALID;
