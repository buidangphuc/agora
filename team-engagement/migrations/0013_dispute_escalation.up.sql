ALTER TABLE disputes ADD COLUMN IF NOT EXISTS return_id VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE disputes ADD COLUMN IF NOT EXISTS ruling INTEGER NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX IF NOT EXISTS uq_disputes_open_return
    ON disputes (return_id) WHERE return_id <> '' AND status IN ('OPEN', 'INVESTIGATING');
CREATE INDEX IF NOT EXISTS idx_disputes_status_created ON disputes (status, created_at, id);
