-- 0011_order_outbox_v1_switch -- outbox upgrade step 3 (outbox.UpgradeSQL Switch).
--
-- One transaction: block writers to the table, number the rows a v0 writer
-- inserted since the last backfill batch (in the same order), and give new rows a
-- number by default. No row is left without a seq afterwards. Writers wait on the
-- lock for as long as that takes, normally a moment, and then get a seq from the
-- new default.
BEGIN;
LOCK TABLE order_outbox_events IN SHARE ROW EXCLUSIVE MODE;
WITH b AS MATERIALIZED (
    SELECT event_id, nextval('order_outbox_events_seq_seq') AS n FROM (
        SELECT event_id FROM order_outbox_events
        WHERE seq IS NULL
        ORDER BY created_at, event_id
    ) x
)
UPDATE order_outbox_events t SET seq = b.n FROM b WHERE t.event_id = b.event_id;
ALTER TABLE order_outbox_events ALTER COLUMN seq SET DEFAULT nextval('order_outbox_events_seq_seq');
COMMIT;
