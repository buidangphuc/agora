DROP INDEX IF EXISTS ad_campaigns_serving_idx;
DROP TABLE IF EXISTS ad_impressions;
ALTER TABLE ad_campaigns DROP COLUMN IF EXISTS category_id;
ALTER TABLE ad_campaigns DROP COLUMN IF EXISTS clicks;
ALTER TABLE ad_campaigns DROP COLUMN IF EXISTS impressions;
ALTER TABLE ad_campaigns DROP COLUMN IF EXISTS spent;
