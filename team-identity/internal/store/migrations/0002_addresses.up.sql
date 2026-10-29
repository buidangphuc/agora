-- 0002_addresses — User shipping address book (Phase 4).

CREATE TABLE IF NOT EXISTS user_addresses (
    id             TEXT PRIMARY KEY,
    user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_name TEXT NOT NULL,
    phone          TEXT NOT NULL,
    street         TEXT NOT NULL,
    ward           TEXT NOT NULL DEFAULT '',
    district       TEXT NOT NULL DEFAULT '',
    city           TEXT NOT NULL,
    is_default     BOOLEAN NOT NULL DEFAULT false,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS user_addresses_user_idx ON user_addresses (user_id);
