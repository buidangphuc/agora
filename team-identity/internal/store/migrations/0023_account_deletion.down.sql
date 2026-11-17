DROP INDEX IF EXISTS users_erase_after_idx;

UPDATE users SET status = 'banned' WHERE status = 'deleted';

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_status_check;
ALTER TABLE users ADD CONSTRAINT users_status_check
    CHECK (status IN ('active', 'restricted', 'suspended', 'banned'));

ALTER TABLE users
    DROP COLUMN IF EXISTS erase_after,
    DROP COLUMN IF EXISTS deletion_requested_at;
