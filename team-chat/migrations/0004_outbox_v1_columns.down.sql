ALTER TABLE chat_outbox_events DROP COLUMN IF EXISTS seq;
ALTER TABLE chat_outbox_events DROP COLUMN IF EXISTS traceparent;
ALTER TABLE chat_outbox_events DROP COLUMN IF EXISTS topic;
DROP SEQUENCE IF EXISTS chat_outbox_events_seq_seq;
