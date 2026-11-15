ALTER TABLE users
    ADD COLUMN IF NOT EXISTS deletion_requested_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS erase_after TIMESTAMPTZ;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_status_check;
ALTER TABLE users ADD CONSTRAINT users_status_check
    CHECK (status IN ('active', 'restricted', 'suspended', 'banned', 'deleted'));

CREATE INDEX IF NOT EXISTS users_erase_after_idx ON users (erase_after) WHERE erase_after IS NOT NULL;
