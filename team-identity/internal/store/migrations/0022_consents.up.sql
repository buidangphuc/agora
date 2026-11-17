CREATE TABLE IF NOT EXISTS consents (
    id          BIGSERIAL PRIMARY KEY,
    user_id     TEXT        NOT NULL REFERENCES users (id),
    purpose     TEXT        NOT NULL,
    granted     BOOLEAN     NOT NULL,
    version     TEXT        NOT NULL,
    source      TEXT        NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS consents_user_recorded_idx
    ON consents (user_id, recorded_at DESC, id DESC);
