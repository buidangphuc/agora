-- Short shareable links with UTM tracking and Open Graph preview metadata.
-- One row per short code; owned by team-sharing.
CREATE TABLE IF NOT EXISTS share_links (
    short_code     TEXT PRIMARY KEY,
    target_type    TEXT        NOT NULL,
    target_id      TEXT        NOT NULL,
    utm            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    og_title       TEXT        NOT NULL DEFAULT '',
    og_description TEXT        NOT NULL DEFAULT '',
    og_image_url   TEXT        NOT NULL DEFAULT '',
    click_count    BIGINT      NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Resolve-by-target lookups (e.g. "does this listing already have a link?").
CREATE INDEX IF NOT EXISTS idx_share_links_target
    ON share_links (target_type, target_id);
