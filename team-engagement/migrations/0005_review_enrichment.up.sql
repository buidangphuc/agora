-- 0005_review_enrichment.up.sql — Richer reviews (F4).
-- Adds media, per-user "helpful" votes, verified-purchase + seller denormalized
-- onto the review row (seller_id is captured from team-order at create time so
-- GetShopRatingSummary can aggregate without a cross-DB join).

ALTER TABLE reviews ADD COLUMN IF NOT EXISTS media_urls        TEXT[]      NOT NULL DEFAULT '{}';
ALTER TABLE reviews ADD COLUMN IF NOT EXISTS helpful_count     BIGINT      NOT NULL DEFAULT 0;
ALTER TABLE reviews ADD COLUMN IF NOT EXISTS verified_purchase BOOLEAN     NOT NULL DEFAULT FALSE;
ALTER TABLE reviews ADD COLUMN IF NOT EXISTS seller_id         VARCHAR(64) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_reviews_seller_id ON reviews (seller_id, created_at DESC);

-- One row per (review, user): the PK makes MarkReviewHelpful idempotent per user.
CREATE TABLE IF NOT EXISTS review_helpful_votes (
    review_id  VARCHAR(64) NOT NULL REFERENCES reviews (id) ON DELETE CASCADE,
    user_id    VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (review_id, user_id)
);
