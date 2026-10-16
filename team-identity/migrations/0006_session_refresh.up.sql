-- 0006_session_refresh — rotating refresh tokens bound to a session.
--
-- Only SHA-256 hashes are stored, never the raw token. All columns are nullable so
-- legacy (pre-refresh) sessions stay valid and the migration is safe on a 0005 DB.
-- refresh_hash is the live token; prev_refresh_hash is the one before the last
-- rotation (grace replay / reuse detection); rotated_at is when that rotation ran.

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS refresh_hash       TEXT;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS prev_refresh_hash  TEXT;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS refresh_expires_at TIMESTAMPTZ;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS rotated_at         TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS sessions_refresh_hash_uidx ON sessions (refresh_hash);
CREATE INDEX IF NOT EXISTS sessions_prev_refresh_hash_idx ON sessions (prev_refresh_hash);
