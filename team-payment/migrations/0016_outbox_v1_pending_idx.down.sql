-- 0016 down — one statement per file (CONCURRENTLY).
DROP INDEX CONCURRENTLY IF EXISTS payment_outbox_events_pending_idx;
