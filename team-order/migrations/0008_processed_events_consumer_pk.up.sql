-- 0008_processed_events_consumer_pk -- the dedupe ledger is keyed per consumer.
--
-- 0004 keyed processed_events by event_id alone and its reader ignored the
-- consumer column, so two consumers could never process the same event id. The
-- go-sdk ledger (consume.Dedupe) records (consumer, event_id) in the same
-- transaction as the handler's writes. Rows written before this migration all
-- belong to the one consumer that existed, team-order.payment (a row may carry an
-- empty consumer, the 0004 column default), so they are backfilled to it: a
-- redelivered event is still recognised as processed after the key changes.
--
-- Idempotent: re-running it on a database that already has the new key is a no-op.
-- Rollout: pods running the previous release insert with ON CONFLICT (event_id),
-- which this key no longer supports, so apply it with the release that replaces
-- them.

UPDATE processed_events SET consumer = 'team-order.payment' WHERE consumer = '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_index i
        JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY (i.indkey)
        WHERE i.indrelid = 'processed_events'::regclass
          AND i.indisprimary
          AND a.attname = 'consumer'
    ) THEN
        ALTER TABLE processed_events DROP CONSTRAINT IF EXISTS processed_events_pkey;
        ALTER TABLE processed_events ADD PRIMARY KEY (consumer, event_id);
    END IF;
END
$$;
