-- 0013: number the existing rows, oldest first, from the sequence: (created_at,
-- event_id) order, 10000 rows per statement, repeated until a statement numbers
-- none. This is outbox.UpgradeSQL's Backfill statement in a loop. Each batch
-- locks its rows, so a second run cannot number a row twice or out of order.
-- The loop runs inside this migration's one transaction, which is fine for the
-- small table of unpublished rows (published rows are purged after 7 days and
-- need no ordered numbers; README says to purge them before the upgrade).
DO $$
DECLARE
    n bigint;
BEGIN
    LOOP
        WITH b AS MATERIALIZED (SELECT event_id, nextval('engagement_outbox_events_seq_seq') AS n FROM
               (SELECT event_id FROM engagement_outbox_events WHERE seq IS NULL ORDER BY created_at, event_id LIMIT 10000 FOR UPDATE) x),
             u AS (UPDATE engagement_outbox_events t SET seq = b.n FROM b WHERE t.event_id = b.event_id AND t.seq IS NULL RETURNING 1)
        SELECT count(*) INTO n FROM u;
        EXIT WHEN n = 0;
    END LOOP;
END $$;
