-- 0007_outbox_v1_backfill — step 2 of 8 (outbox.UpgradeSQL Backfill). Numbers the
-- existing rows by (created_at, event_id), 10000 at a time, from the sequence, until
-- no row is left without a seq. Each batch locks its rows, so two runs at once cannot
-- number a row twice or out of order. The loop is one statement and so one
-- transaction here; purge published rows before running it (every batch sorts all
-- rows that are still unnumbered), which keeps the outbox small enough for that.

DO $$
DECLARE
    updated bigint;
BEGIN
    LOOP
        WITH b AS MATERIALIZED (SELECT event_id, nextval('identity_outbox_events_seq_seq') AS n FROM
               (SELECT event_id FROM identity_outbox_events WHERE seq IS NULL ORDER BY created_at, event_id LIMIT 10000 FOR UPDATE) x),
             u AS (UPDATE identity_outbox_events t SET seq = b.n FROM b WHERE t.event_id = b.event_id AND t.seq IS NULL RETURNING 1)
        SELECT count(*) INTO updated FROM u;
        EXIT WHEN updated = 0;
    END LOOP;
END
$$;
