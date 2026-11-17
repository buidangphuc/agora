ALTER TABLE ad_campaigns ADD COLUMN IF NOT EXISTS spent BIGINT NOT NULL DEFAULT 0;
ALTER TABLE ad_campaigns ADD COLUMN IF NOT EXISTS impressions BIGINT NOT NULL DEFAULT 0;
ALTER TABLE ad_campaigns ADD COLUMN IF NOT EXISTS clicks BIGINT NOT NULL DEFAULT 0;
ALTER TABLE ad_campaigns ADD COLUMN IF NOT EXISTS category_id TEXT NOT NULL DEFAULT '';

UPDATE ad_campaigns SET status = 3 WHERE status = 1 AND (bid < 500 OR budget < bid);

CREATE TABLE IF NOT EXISTS ad_impressions (
    token       TEXT PRIMARY KEY,
    campaign_id TEXT NOT NULL REFERENCES ad_campaigns (id),
    listing_id  TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    clicked_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS ad_impressions_campaign_idx ON ad_impressions (campaign_id);
CREATE INDEX IF NOT EXISTS ad_campaigns_serving_idx ON ad_campaigns (status, bid DESC, created_at);
