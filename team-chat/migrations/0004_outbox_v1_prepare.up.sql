-- go-sdk v1 outbox upgrade, step 1 of 8 (outbox.UpgradeSQL Prepare): the columns v1
-- needs and the sequence that numbers rows. Instant, no table rewrite. seq is a
-- plain BIGINT here, not BIGSERIAL: BIGSERIAL would rewrite the table under ACCESS
-- EXCLUSIVE and number the old rows in heap order, so a retried row would move
-- behind newer events of its aggregate.
--
-- Deploy order: stop the v0 relayers (or let them drain), requeue or delete rows a
-- v0 relayer parked as failed, purge published rows, then run these migrations.
-- See the README "Deploying the go-sdk v1 outbox" section.
ALTER TABLE chat_outbox_events ADD COLUMN IF NOT EXISTS topic       TEXT;
ALTER TABLE chat_outbox_events ADD COLUMN IF NOT EXISTS traceparent TEXT;
ALTER TABLE chat_outbox_events ADD COLUMN IF NOT EXISTS seq         BIGINT;
CREATE SEQUENCE IF NOT EXISTS chat_outbox_events_seq_seq OWNED BY chat_outbox_events.seq;
