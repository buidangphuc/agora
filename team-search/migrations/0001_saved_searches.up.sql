-- 0001_saved_searches — user-scoped saved searches (F2 Saved Searches).
-- Personal state that sits alongside the OpenSearch read-model: a parked
-- query + filters the owner can re-run later. Self-scoped by user_id.

CREATE TABLE IF NOT EXISTS saved_searches (
    id         TEXT PRIMARY KEY,
    user_id    TEXT        NOT NULL,
    query      TEXT        NOT NULL DEFAULT '',
    filters    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS saved_searches_user_idx
    ON saved_searches (user_id, created_at DESC);
