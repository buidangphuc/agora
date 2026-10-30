ALTER TABLE order_outbox_events ADD COLUMN IF NOT EXISTS topic       TEXT;
ALTER TABLE order_outbox_events ADD COLUMN IF NOT EXISTS traceparent TEXT;
ALTER TABLE order_outbox_events ADD COLUMN IF NOT EXISTS seq         BIGINT;
CREATE SEQUENCE IF NOT EXISTS order_outbox_events_seq_seq OWNED BY order_outbox_events.seq;
