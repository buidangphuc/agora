-- team-engagement OWNS engagement_db (Rule 3).
CREATE TABLE IF NOT EXISTS favorites (
    user_id    TEXT        NOT NULL,
    listing_id TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, listing_id)
);
CREATE INDEX IF NOT EXISTS favorites_user_idx ON favorites (user_id);

CREATE TABLE IF NOT EXISTS listing_stats (
    listing_id     TEXT   PRIMARY KEY,
    view_count     BIGINT NOT NULL DEFAULT 0,
    favorite_count BIGINT NOT NULL DEFAULT 0
);
