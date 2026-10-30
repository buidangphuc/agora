ALTER TABLE order_outbox_events DROP COLUMN IF EXISTS seq;
ALTER TABLE order_outbox_events DROP COLUMN IF EXISTS traceparent;
ALTER TABLE order_outbox_events DROP COLUMN IF EXISTS topic;
DROP SEQUENCE IF EXISTS order_outbox_events_seq_seq;
