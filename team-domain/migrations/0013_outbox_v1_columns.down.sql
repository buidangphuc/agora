DROP SEQUENCE IF EXISTS outbox_events_seq_seq CASCADE;
ALTER TABLE outbox_events DROP COLUMN IF EXISTS seq;
ALTER TABLE outbox_events DROP COLUMN IF EXISTS traceparent;
ALTER TABLE outbox_events DROP COLUMN IF EXISTS topic;
