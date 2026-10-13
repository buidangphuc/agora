-- 0008_outbox_v1_switch — step 3 of 8 (outbox.UpgradeSQL Switch), ONE transaction:
-- block writers, number the rows a v0 writer inserted since the backfill, give new
-- rows a number by default. No row is left without a seq afterwards. Inserts into the
-- table wait for the lock for as long as the numbering takes, normally a moment.

BEGIN;
LOCK TABLE identity_outbox_events IN SHARE ROW EXCLUSIVE MODE;
WITH b AS MATERIALIZED (SELECT event_id, nextval('identity_outbox_events_seq_seq') AS n FROM
       (SELECT event_id FROM identity_outbox_events WHERE seq IS NULL ORDER BY created_at, event_id) x)
UPDATE identity_outbox_events t SET seq = b.n FROM b WHERE t.event_id = b.event_id;
ALTER TABLE identity_outbox_events ALTER COLUMN seq SET DEFAULT nextval('identity_outbox_events_seq_seq');
COMMIT;
