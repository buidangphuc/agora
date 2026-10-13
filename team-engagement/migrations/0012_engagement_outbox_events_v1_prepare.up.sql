-- 0012: first step of upgrading engagement_outbox_events from the go-sdk v0 shape
-- to the v1 shape (go-sdk/v1.0.0, outbox.UpgradeSQL("engagement_outbox_events")).
-- Forward-only. The steps are 0012 prepare, 0013 backfill, 0014 switch,
-- 0015 not-null, 0016 to 0018 indexes, 0019 drop of the v0 claim index; README
-- "Outbox upgrade" says what to do before deploying them.
--
-- Adds the columns and the sequence. Instant, no table rewrite. seq is NOT added as
-- BIGSERIAL: that would rewrite the table under ACCESS EXCLUSIVE and number the
-- old rows in heap order, which moves a retried row behind newer events of its
-- aggregate.
ALTER TABLE engagement_outbox_events ADD COLUMN IF NOT EXISTS topic       TEXT;
ALTER TABLE engagement_outbox_events ADD COLUMN IF NOT EXISTS traceparent TEXT;
ALTER TABLE engagement_outbox_events ADD COLUMN IF NOT EXISTS seq         BIGINT;
CREATE SEQUENCE IF NOT EXISTS engagement_outbox_events_seq_seq OWNED BY engagement_outbox_events.seq;
