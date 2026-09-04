-- 0007_follows.up.sql — Seller follow graph + follow-feed source (F1).
-- team-engagement OWNS engagement_db (Rule 3).

-- follows: one row per (user_id, seller_id). The PRIMARY KEY makes the pair
-- unique so a repeat FollowSeller is idempotent (ON CONFLICT DO NOTHING).
CREATE TABLE IF NOT EXISTS follows (
    user_id    TEXT        NOT NULL,
    seller_id  TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, seller_id)
);
CREATE INDEX IF NOT EXISTS follows_user_idx ON follows (user_id);
CREATE INDEX IF NOT EXISTS follows_seller_idx ON follows (seller_id);

-- seller_listings: the follow-feed source — which listings belong to which
-- seller. ListFollowedListings joins follows → seller_listings to return the
-- listing ids the feed resolves. Populated from listing events in the
-- integration wave (engagement does not own listings); the join and query are
-- real here so the RPC works against whatever rows exist.
CREATE TABLE IF NOT EXISTS seller_listings (
    seller_id  TEXT        NOT NULL,
    listing_id TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (seller_id, listing_id)
);
CREATE INDEX IF NOT EXISTS seller_listings_seller_idx ON seller_listings (seller_id, listing_id);
