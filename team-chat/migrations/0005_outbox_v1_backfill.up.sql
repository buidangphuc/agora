-- go-sdk v1 outbox upgrade, step 2 of 8 (outbox.UpgradeSQL Backfill): number the
-- existing rows by (created_at, event_id), 10000 at a time, until none is left.
-- Numbers come from the sequence in that order and each batch locks its rows, so a
-- concurrent run cannot number a row twice or out of order. Do not run this again
-- after 0006.
DO $$
DECLARE
    n BIGINT;
BEGIN
    LOOP
        WITH b AS MATERIALIZED (
            SELECT event_id, nextval('chat_outbox_events_seq_seq') AS n FROM
                (SELECT event_id FROM chat_outbox_events WHERE seq IS NULL ORDER BY created_at, event_id LIMIT 10000 FOR UPDATE) x),
        u AS (UPDATE chat_outbox_events t SET seq = b.n FROM b WHERE t.event_id = b.event_id AND t.seq IS NULL RETURNING 1)
        SELECT count(*) INTO n FROM u;
        EXIT WHEN n = 0;
    END LOOP;
END
$$;
