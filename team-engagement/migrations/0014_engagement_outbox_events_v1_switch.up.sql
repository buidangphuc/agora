-- 0014: switch, in one transaction. Blocks writers to the table for a moment,
-- numbers the rows a v0 writer inserted since the last backfill batch the same
-- way, and gives new rows a number by default. Afterwards no row is without a seq.
-- Never run the backfill again after this.
BEGIN;
LOCK TABLE engagement_outbox_events IN SHARE ROW EXCLUSIVE MODE;
WITH b AS MATERIALIZED (SELECT event_id, nextval('engagement_outbox_events_seq_seq') AS n FROM
       (SELECT event_id FROM engagement_outbox_events WHERE seq IS NULL ORDER BY created_at, event_id) x)
UPDATE engagement_outbox_events t SET seq = b.n FROM b WHERE t.event_id = b.event_id;
ALTER TABLE engagement_outbox_events ALTER COLUMN seq SET DEFAULT nextval('engagement_outbox_events_seq_seq');
COMMIT;
