DO $do$
DECLARE
    updated bigint;
BEGIN
    LOOP
        EXECUTE $q$WITH b AS MATERIALIZED (SELECT event_id, nextval('outbox_events_seq_seq') AS n FROM
  (SELECT event_id FROM outbox_events WHERE seq IS NULL ORDER BY created_at, event_id LIMIT 10000 FOR UPDATE) x),
u AS (UPDATE outbox_events t SET seq = b.n FROM b WHERE t.event_id = b.event_id AND t.seq IS NULL RETURNING 1)
SELECT count(*) AS updated FROM u$q$ INTO updated;
        EXIT WHEN updated = 0;
    END LOOP;
END
$do$;
