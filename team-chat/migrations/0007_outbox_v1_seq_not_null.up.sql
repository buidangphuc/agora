ALTER TABLE chat_outbox_events DROP CONSTRAINT IF EXISTS chat_outbox_events_seq_nn;
ALTER TABLE chat_outbox_events ADD CONSTRAINT chat_outbox_events_seq_nn CHECK (seq IS NOT NULL) NOT VALID;
ALTER TABLE chat_outbox_events VALIDATE CONSTRAINT chat_outbox_events_seq_nn;
ALTER TABLE chat_outbox_events ALTER COLUMN seq SET NOT NULL;
ALTER TABLE chat_outbox_events DROP CONSTRAINT chat_outbox_events_seq_nn;
