-- 0008_storefronts — F5 Seller storefront config (team-domain owns it).
--
-- One row per seller: their public shop page. seller_id is the primary key
-- (a seller has at most one storefront), slug is a globally-unique handle used
-- for pretty URLs (/shop/<slug>), and config holds the presentational fields
-- (banner_url, tagline, featured_listing_ids, theme) as JSONB so the shape can
-- evolve without a migration. UpsertStorefront is owner-scoped: a seller only
-- ever writes the row keyed by their own principal id.

CREATE TABLE IF NOT EXISTS storefronts (
    seller_id  TEXT        PRIMARY KEY,               -- owner principal id (server-forced)
    slug       TEXT        NOT NULL,                  -- unique public handle
    config     JSONB       NOT NULL DEFAULT '{}',     -- banner_url, tagline, featured_listing_ids, theme
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Slug is a globally-unique handle: two sellers cannot claim the same one.
CREATE UNIQUE INDEX IF NOT EXISTS storefronts_slug_key ON storefronts (slug);
