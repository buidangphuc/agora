-- 0008_outbox_v1_prepare — go-sdk v1 outbox, step 1 of 10 (outbox.UpgradeSQL Prepare).
--
-- v1 orders events per aggregate by a `seq` number and can publish to a per-row topic with a
-- stored traceparent. A table made by 0003 lacks these columns. Instant, no table rewrite.
-- Deploy order is in the README ("Upgrading the outbox table to go-sdk v1"): stop the v0
-- relayer first, run 0008..0017 with the migrate job, then start v1.
--
-- Do NOT add seq as BIGSERIAL: that rewrites the table under ACCESS EXCLUSIVE and numbers old
-- rows in heap order, so a retried (rewritten) row would move behind newer events of its
-- aggregate. 0009 numbers them by (created_at, event_id) instead.
ALTER TABLE payment_outbox_events ADD COLUMN IF NOT EXISTS topic       TEXT;
ALTER TABLE payment_outbox_events ADD COLUMN IF NOT EXISTS traceparent TEXT;
ALTER TABLE payment_outbox_events ADD COLUMN IF NOT EXISTS seq         BIGINT;
CREATE SEQUENCE IF NOT EXISTS payment_outbox_events_seq_seq OWNED BY payment_outbox_events.seq;
