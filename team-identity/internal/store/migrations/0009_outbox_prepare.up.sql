ALTER TABLE identity_outbox_events ADD COLUMN IF NOT EXISTS topic       TEXT;
ALTER TABLE identity_outbox_events ADD COLUMN IF NOT EXISTS traceparent TEXT;
ALTER TABLE identity_outbox_events ADD COLUMN IF NOT EXISTS seq         BIGINT;
CREATE SEQUENCE IF NOT EXISTS identity_outbox_events_seq_seq OWNED BY identity_outbox_events.seq;
