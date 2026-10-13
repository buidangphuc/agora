-- 0011_outbox_v1_columns: first step of moving outbox_events to the go-sdk v1
-- outbox shape (outbox.UpgradeSQL at go-sdk/v1.0.0, "Prepare"). Adds the
-- columns the v1 relayer reads and the sequence that will order the events.
-- Instant: no table rewrite.
--
-- seq is NOT added as BIGSERIAL: that would rewrite the table under ACCESS
-- EXCLUSIVE and number the old rows in heap order, so a retried row (rewritten
-- by an UPDATE) would move behind newer events of its aggregate. The next two
-- migrations number the old rows by (created_at, event_id) instead.
--
-- Deploy order (see README "Deploying the outbox v1 upgrade"): stop the v0
-- relayers first, deal with `failed` rows, purge published rows, then run
-- migrations 0011-0018, then start the v1 service.
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS topic       TEXT;
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS traceparent TEXT;
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS seq         BIGINT;
CREATE SEQUENCE IF NOT EXISTS outbox_events_seq_seq OWNED BY outbox_events.seq;
