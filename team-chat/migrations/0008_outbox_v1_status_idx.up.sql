-- go-sdk v1 outbox upgrade, step 5 of 8. CONCURRENTLY cannot run inside a
-- transaction, so this file holds exactly one statement (golang-migrate runs a
-- file as one Exec; with a single statement there is no implicit transaction). If
-- it fails, DROP the invalid index and rerun.
CREATE INDEX CONCURRENTLY IF NOT EXISTS chat_outbox_events_status_idx ON chat_outbox_events (status);
