-- 0008_session_revoked_at — when a session was revoked, for retention purging.
--
-- Nullable: legacy revoked rows keep NULL. Re-runnable (IF NOT EXISTS) like 0006/0007.

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS sessions_revoked_at_idx ON sessions (revoked_at) WHERE revoked AND revoked_at IS NOT NULL;
