-- 0006_view_history.up.sql — Per-user recently-viewed history.
-- One row per (user_id, listing_id); a repeat view bumps viewed_at (dedup,
-- most-recent-first). Anonymous views are not recorded here.

CREATE TABLE IF NOT EXISTS view_history (
    user_id    TEXT        NOT NULL,
    listing_id TEXT        NOT NULL,
    viewed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, listing_id)
);

CREATE INDEX IF NOT EXISTS view_history_user_recent_idx ON view_history (user_id, viewed_at DESC);
