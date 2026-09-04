-- 0009_bundles — F5 Bundle deals (team-domain owns it).
--
-- A bundle groups several of a seller's listings sold together at a single
-- bundle_price. One row per bundle; seller_id is the owner principal id
-- (server-forced on create, never client-supplied), listing_ids holds the
-- member listings as a text[] and bundle_price is minor units (e.g. VND).
-- Mirrors the listings table: server-assigned id, owner column, created_at.

CREATE TABLE IF NOT EXISTS bundles (
    id           TEXT        PRIMARY KEY,               -- server-assigned bundle id
    seller_id    TEXT        NOT NULL,                  -- owner principal id (server-forced)
    title        TEXT        NOT NULL,                  -- human label for the bundle
    listing_ids  TEXT[]      NOT NULL DEFAULT '{}',     -- member listing ids
    bundle_price BIGINT      NOT NULL DEFAULT 0,        -- minor units (e.g. VND)
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ListBundlesBySeller pages a seller's bundles ordered by id; index the owner.
CREATE INDEX IF NOT EXISTS bundles_seller_id_idx ON bundles (seller_id, id);
