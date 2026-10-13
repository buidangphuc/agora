-- 0009_outbox_v1_backfill — step 2 of 10 (outbox.UpgradeSQL Backfill).
--
-- Numbers the existing rows by (created_at, event_id), 10000 at a time, until no row is left
-- without a seq. Numbers come from the sequence in that order and each batch locks its rows, so
-- two runs at once cannot number a row twice or out of order. This runs as one statement (one
-- transaction), which is fine for an outbox this small; purge published rows before upgrading
-- (README) so the sort stays cheap. Do not run it again after 0010: no row is left without a seq.
DO $$
DECLARE
    updated bigint;
BEGIN
    LOOP
        WITH b AS MATERIALIZED (SELECT event_id, nextval('payment_outbox_events_seq_seq') AS n FROM
               (SELECT event_id FROM payment_outbox_events WHERE seq IS NULL ORDER BY created_at, event_id LIMIT 10000 FOR UPDATE) x),
             u AS (UPDATE payment_outbox_events t SET seq = b.n FROM b WHERE t.event_id = b.event_id AND t.seq IS NULL RETURNING 1)
        SELECT count(*) INTO updated FROM u;
        EXIT WHEN updated = 0;
    END LOOP;
END
$$;
