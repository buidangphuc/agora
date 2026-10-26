ALTER TABLE payment_outbox_events ADD COLUMN IF NOT EXISTS topic       TEXT;
ALTER TABLE payment_outbox_events ADD COLUMN IF NOT EXISTS traceparent TEXT;
ALTER TABLE payment_outbox_events ADD COLUMN IF NOT EXISTS seq         BIGINT;
CREATE SEQUENCE IF NOT EXISTS payment_outbox_events_seq_seq OWNED BY payment_outbox_events.seq;
