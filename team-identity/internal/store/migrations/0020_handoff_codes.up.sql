CREATE TABLE IF NOT EXISTS handoff_codes (
    code_hash   BYTEA PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_sid  TEXT NOT NULL,
    target_app  TEXT NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    redeemed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS handoff_codes_expires_at_idx ON handoff_codes (expires_at);
CREATE INDEX IF NOT EXISTS handoff_codes_user_id_idx ON handoff_codes (user_id, expires_at);
