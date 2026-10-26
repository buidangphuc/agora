LOCK TABLE outbox_events IN SHARE ROW EXCLUSIVE MODE;
WITH b AS MATERIALIZED (SELECT event_id, nextval('outbox_events_seq_seq') AS n FROM
  (SELECT event_id FROM outbox_events WHERE seq IS NULL ORDER BY created_at, event_id) x)
UPDATE outbox_events t SET seq = b.n FROM b WHERE t.event_id = b.event_id;
ALTER TABLE outbox_events ALTER COLUMN seq SET DEFAULT nextval('outbox_events_seq_seq');
