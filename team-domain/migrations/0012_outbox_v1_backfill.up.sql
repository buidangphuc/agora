-- 0012_outbox_v1_backfill: number the existing rows by (created_at, event_id),
-- 10000 at a time, until none is left (outbox.UpgradeSQL "Backfill"). Numbers
-- come from the sequence in that order and each batch locks its rows, so a
-- concurrent run cannot number a row twice or out of order.
--
-- A DO block is one transaction, so the row locks are held until it ends. That
-- is fine for a table that was purged of published rows (the deploy note says
-- to) and has no v0 relayer running.
DO $$
DECLARE
    n bigint;
BEGIN
    LOOP
        WITH b AS MATERIALIZED (
            SELECT event_id, nextval('outbox_events_seq_seq') AS n FROM
                (SELECT event_id FROM outbox_events WHERE seq IS NULL
                 ORDER BY created_at, event_id LIMIT 10000 FOR UPDATE) x),
        u AS (
            UPDATE outbox_events t SET seq = b.n FROM b
            WHERE t.event_id = b.event_id AND t.seq IS NULL RETURNING 1)
        SELECT count(*) INTO n FROM u;
        EXIT WHEN n = 0;
    END LOOP;
END
$$;
