-- 0004_wishlist_collections.up.sql — Named wishlist collections (F3).
-- Collections are per-user named lists; items are (collection, listing) pairs.

CREATE TABLE IF NOT EXISTS collections (
    id          VARCHAR(64) PRIMARY KEY,
    user_id     VARCHAR(64) NOT NULL,
    name        VARCHAR(128) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_collections_user_id ON collections (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS collection_items (
    collection_id VARCHAR(64) NOT NULL REFERENCES collections (id) ON DELETE CASCADE,
    listing_id    VARCHAR(64) NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (collection_id, listing_id)
);

CREATE INDEX IF NOT EXISTS idx_collection_items_collection ON collection_items (collection_id, created_at DESC);
