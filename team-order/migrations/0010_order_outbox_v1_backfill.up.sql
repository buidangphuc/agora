-- 0010_order_outbox_v1_backfill -- outbox upgrade step 2 (outbox.UpgradeSQL Backfill).
--
-- Numbers the rows that have no seq yet, oldest first by (created_at, event_id),
-- 10000 at a time from the sequence, until a batch updates nothing. Each batch
-- locks its rows, so two runs cannot number a row twice or out of order. Purging
-- published rows before the deploy keeps this fast: every batch sorts all
-- still-unnumbered rows.
DO $$
DECLARE
    updated bigint;
BEGIN
    LOOP
        WITH b AS MATERIALIZED (
            SELECT event_id, nextval('order_outbox_events_seq_seq') AS n FROM (
                SELECT event_id FROM order_outbox_events
                WHERE seq IS NULL
                ORDER BY created_at, event_id
                LIMIT 10000
                FOR UPDATE
            ) x
        ),
        u AS (
            UPDATE order_outbox_events t SET seq = b.n
            FROM b
            WHERE t.event_id = b.event_id AND t.seq IS NULL
            RETURNING 1
        )
        SELECT count(*) INTO updated FROM u;
        EXIT WHEN updated = 0;
    END LOOP;
END
$$;
