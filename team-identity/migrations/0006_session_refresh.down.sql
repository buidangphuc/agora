DROP INDEX IF EXISTS sessions_prev_refresh_hash_idx;
DROP INDEX IF EXISTS sessions_refresh_hash_uidx;
ALTER TABLE sessions DROP COLUMN IF EXISTS rotated_at;
ALTER TABLE sessions DROP COLUMN IF EXISTS refresh_expires_at;
ALTER TABLE sessions DROP COLUMN IF EXISTS prev_refresh_hash;
ALTER TABLE sessions DROP COLUMN IF EXISTS refresh_hash;
