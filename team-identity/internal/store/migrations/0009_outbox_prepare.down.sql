DROP SEQUENCE IF EXISTS identity_outbox_events_seq_seq;
ALTER TABLE identity_outbox_events DROP COLUMN IF EXISTS seq;
ALTER TABLE identity_outbox_events DROP COLUMN IF EXISTS traceparent;
ALTER TABLE identity_outbox_events DROP COLUMN IF EXISTS topic;
