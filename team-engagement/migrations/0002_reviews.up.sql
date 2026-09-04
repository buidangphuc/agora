-- 0002_reviews.up.sql — Schema for Reviews & Ratings (Phase 6).

CREATE TABLE IF NOT EXISTS reviews (
    id          VARCHAR(64) PRIMARY KEY,
    listing_id  VARCHAR(64) NOT NULL,
    user_id     VARCHAR(64) NOT NULL,
    user_name   VARCHAR(128) NOT NULL DEFAULT '',
    order_id    VARCHAR(64) NOT NULL DEFAULT '',
    rating      INT NOT NULL CHECK (rating >= 1 AND rating <= 5),
    comment     TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_reviews_listing_id ON reviews (listing_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_reviews_user_id ON reviews (user_id);
