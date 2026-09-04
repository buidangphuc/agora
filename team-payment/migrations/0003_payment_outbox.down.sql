-- 0003_payment_outbox down — drop the payment transactional outbox.
DROP INDEX IF EXISTS payment_outbox_events_status_idx;
DROP INDEX IF EXISTS payment_outbox_events_claim_idx;
DROP TABLE IF EXISTS payment_outbox_events;
