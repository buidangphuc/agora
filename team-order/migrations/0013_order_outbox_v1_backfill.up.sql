DO $$
DECLARE n bigint;
BEGIN
    LOOP
        WITH b AS MATERIALIZED (SELECT event_id, nextval('order_outbox_events_seq_seq') AS n FROM
          (SELECT event_id FROM order_outbox_events WHERE seq IS NULL ORDER BY created_at, event_id LIMIT 10000 FOR UPDATE) x),
        u AS (UPDATE order_outbox_events t SET seq = b.n FROM b WHERE t.event_id = b.event_id AND t.seq IS NULL RETURNING 1)
        SELECT count(*) INTO n FROM u;
        EXIT WHEN n = 0;
    END LOOP;
END
$$;

