-- 0012_reports.up.sql — listing reports (engagement-listing-reports).

CREATE TABLE IF NOT EXISTS reports (
    id              VARCHAR(64) PRIMARY KEY,
    target_type     VARCHAR(32) NOT NULL,
    target_id       VARCHAR(64) NOT NULL,
    reason_code     VARCHAR(32) NOT NULL,
    note            TEXT        NOT NULL DEFAULT '',
    reporter_id     VARCHAR(64) NOT NULL,
    status          VARCHAR(16) NOT NULL DEFAULT 'OPEN',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at     TIMESTAMPTZ,
    resolved_by     VARCHAR(64) NOT NULL DEFAULT '',
    resolution_note TEXT        NOT NULL DEFAULT ''
);

-- At most one OPEN report per (reporter, target).
CREATE UNIQUE INDEX IF NOT EXISTS uq_reports_open_reporter_target
    ON reports (reporter_id, target_type, target_id) WHERE status = 'OPEN';
CREATE INDEX IF NOT EXISTS idx_reports_status_created ON reports (status, created_at, id);
CREATE INDEX IF NOT EXISTS idx_reports_target ON reports (target_id);
CREATE INDEX IF NOT EXISTS idx_reports_resolved_at ON reports (resolved_at);
