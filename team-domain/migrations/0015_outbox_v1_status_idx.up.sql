-- One statement per file: CREATE INDEX CONCURRENTLY cannot run inside a
-- transaction, and a multi-statement file is sent as one implicit transaction.
-- If it fails, DROP the invalid index and rerun.
CREATE INDEX CONCURRENTLY IF NOT EXISTS outbox_events_status_idx ON outbox_events (status);
