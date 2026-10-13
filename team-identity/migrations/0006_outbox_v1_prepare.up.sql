-- 0006_outbox_v1_prepare — go-sdk v1 outbox upgrade, step 1 of 8 (outbox.UpgradeSQL
-- Prepare). Adds the columns and the sequence v1 needs; instant, no table rewrite.
--
-- Deploy order (see README "Outbox upgrade"): stop the v0 relayers first, deal with
-- rows a v0 relayer parked as 'failed', purge published rows, THEN run 0006-0013, then
-- deploy v1. Do not add seq as BIGSERIAL: it rewrites the table under ACCESS EXCLUSIVE
-- and numbers old rows in heap order, so a retried row would move behind newer events
-- of its aggregate.

ALTER TABLE identity_outbox_events ADD COLUMN IF NOT EXISTS topic       TEXT;
ALTER TABLE identity_outbox_events ADD COLUMN IF NOT EXISTS traceparent TEXT;
ALTER TABLE identity_outbox_events ADD COLUMN IF NOT EXISTS seq         BIGINT;
CREATE SEQUENCE IF NOT EXISTS identity_outbox_events_seq_seq OWNED BY identity_outbox_events.seq;
