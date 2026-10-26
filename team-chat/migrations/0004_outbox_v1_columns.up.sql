ALTER TABLE chat_outbox_events ADD COLUMN IF NOT EXISTS topic       TEXT;
ALTER TABLE chat_outbox_events ADD COLUMN IF NOT EXISTS traceparent TEXT;
ALTER TABLE chat_outbox_events ADD COLUMN IF NOT EXISTS seq         BIGINT;
CREATE SEQUENCE IF NOT EXISTS chat_outbox_events_seq_seq OWNED BY chat_outbox_events.seq;
