-- 0009_order_outbox_v1_prepare -- go-sdk v1 outbox upgrade, step 1 of 10 (outbox.UpgradeSQL Prepare).
--
-- The v1 outbox relayer publishes the events of one aggregate in order by a seq
-- column, takes a per-row topic and republishes the producer's traceparent. This
-- step adds the columns and the sequence without rewriting the table: adding seq
-- as BIGSERIAL would take ACCESS EXCLUSIVE and number the old rows in heap order,
-- so a row a v0 relayer retried (rewritten by an UPDATE) would move behind newer
-- events of its aggregate. Steps 0010-0014 number the old rows by (created_at,
-- event_id), switch the default and enforce NOT NULL; 0015-0018 build the indexes
-- (one CONCURRENTLY statement per file, because golang-migrate runs a file as one
-- statement and CREATE INDEX CONCURRENTLY cannot run inside a transaction).
--
-- Deploy order: stop the v0 relayers (or let them drain), purge published rows and
-- deal with `failed` rows first (see README "Deploying the outbox upgrade").
ALTER TABLE order_outbox_events ADD COLUMN IF NOT EXISTS topic       TEXT;
ALTER TABLE order_outbox_events ADD COLUMN IF NOT EXISTS traceparent TEXT;
ALTER TABLE order_outbox_events ADD COLUMN IF NOT EXISTS seq         BIGINT;
CREATE SEQUENCE IF NOT EXISTS order_outbox_events_seq_seq OWNED BY order_outbox_events.seq;
