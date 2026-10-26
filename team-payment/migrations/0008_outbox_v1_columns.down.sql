ALTER TABLE payment_outbox_events DROP COLUMN IF EXISTS seq;
ALTER TABLE payment_outbox_events DROP COLUMN IF EXISTS traceparent;
ALTER TABLE payment_outbox_events DROP COLUMN IF EXISTS topic;
DROP SEQUENCE IF EXISTS payment_outbox_events_seq_seq;
