-- 0005_review_enrichment.down.sql

DROP TABLE IF EXISTS review_helpful_votes;

DROP INDEX IF EXISTS idx_reviews_seller_id;

ALTER TABLE reviews DROP COLUMN IF EXISTS seller_id;
ALTER TABLE reviews DROP COLUMN IF EXISTS verified_purchase;
ALTER TABLE reviews DROP COLUMN IF EXISTS helpful_count;
ALTER TABLE reviews DROP COLUMN IF EXISTS media_urls;
