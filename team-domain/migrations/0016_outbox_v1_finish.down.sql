CREATE INDEX IF NOT EXISTS outbox_events_claim_idx ON outbox_events (available_at, created_at) WHERE status = 'pending';
DROP INDEX IF EXISTS outbox_events_pending_idx;
DROP INDEX IF EXISTS outbox_events_agg_idx;
ALTER TABLE outbox_events ALTER COLUMN seq DROP NOT NULL;
