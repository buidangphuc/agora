DROP INDEX IF EXISTS sessions_revoked_at_idx;
ALTER TABLE sessions DROP COLUMN IF EXISTS revoked_at;
