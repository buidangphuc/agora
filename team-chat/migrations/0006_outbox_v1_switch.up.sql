-- go-sdk v1 outbox upgrade, step 3 of 8 (outbox.UpgradeSQL Switch), one transaction:
-- block writers, number the rows a v0 writer inserted since the last batch, and
-- give new rows a number by default. Inserts wait on the lock for as long as it
-- takes to number those few rows, then get a seq from the new default.
BEGIN;
LOCK TABLE chat_outbox_events IN SHARE ROW EXCLUSIVE MODE;
WITH b AS MATERIALIZED (
    SELECT event_id, nextval('chat_outbox_events_seq_seq') AS n FROM
        (SELECT event_id FROM chat_outbox_events WHERE seq IS NULL ORDER BY created_at, event_id) x)
UPDATE chat_outbox_events t SET seq = b.n FROM b WHERE t.event_id = b.event_id;
ALTER TABLE chat_outbox_events ALTER COLUMN seq SET DEFAULT nextval('chat_outbox_events_seq_seq');
COMMIT;
