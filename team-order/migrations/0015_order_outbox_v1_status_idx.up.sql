-- 0015_order_outbox_v1_status_idx -- outbox upgrade step 5a. One statement per file:
-- CONCURRENTLY cannot run inside a transaction, and golang-migrate runs a file as
-- a single statement. If the build fails, DROP the invalid index and run again.
CREATE INDEX CONCURRENTLY IF NOT EXISTS order_outbox_events_status_idx ON order_outbox_events (status);
