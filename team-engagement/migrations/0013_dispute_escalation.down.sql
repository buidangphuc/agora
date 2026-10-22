DROP INDEX IF EXISTS idx_disputes_status_created;
DROP INDEX IF EXISTS uq_disputes_open_return;
ALTER TABLE disputes DROP COLUMN IF EXISTS ruling;
ALTER TABLE disputes DROP COLUMN IF EXISTS return_id;
