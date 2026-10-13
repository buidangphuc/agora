-- 0010_outbox_v1_switch — step 3 of 10 (outbox.UpgradeSQL Switch), ONE transaction.
--
-- Blocks writers briefly, numbers the rows a v0 writer inserted since 0009, and gives new rows
-- a number by default. No row is left without a seq afterwards.
BEGIN;
LOCK TABLE payment_outbox_events IN SHARE ROW EXCLUSIVE MODE;
WITH b AS MATERIALIZED (SELECT event_id, nextval('payment_outbox_events_seq_seq') AS n FROM
       (SELECT event_id FROM payment_outbox_events WHERE seq IS NULL ORDER BY created_at, event_id) x)
UPDATE payment_outbox_events t SET seq = b.n FROM b WHERE t.event_id = b.event_id;
ALTER TABLE payment_outbox_events ALTER COLUMN seq SET DEFAULT nextval('payment_outbox_events_seq_seq');
COMMIT;
