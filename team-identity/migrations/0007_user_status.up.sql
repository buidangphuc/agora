-- 0007_user_status — account enforcement state on users.
--
-- Existing rows read as 'active'. status is limited to the four known values by a
-- CHECK constraint. Re-runnable (IF NOT EXISTS) like 0006.

ALTER TABLE users ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active'
    CONSTRAINT users_status_check CHECK (status IN ('active', 'restricted', 'suspended', 'banned'));
ALTER TABLE users ADD COLUMN IF NOT EXISTS status_reason     TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS status_until      TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS status_changed_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS users_status_idx ON users (status);
