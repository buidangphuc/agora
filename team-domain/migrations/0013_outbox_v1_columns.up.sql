ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS topic       TEXT;
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS traceparent TEXT;
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS seq         BIGINT;
CREATE SEQUENCE IF NOT EXISTS outbox_events_seq_seq OWNED BY outbox_events.seq;
