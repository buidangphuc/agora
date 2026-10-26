-- 0003_sponsored — Sponsored placements / pay-per-slot ad campaigns for
-- team-promotion. Budget/bid are pure bookkeeping (minor units); creating a
-- campaign is MOCK and moves no money (payments/wallet stay mock, AGENTS.md §7).

-- ad_campaigns holds a seller's sponsored-listing campaigns. status is the
-- AdCampaignStatus enum (1 active, 2 paused, 3 ended). ListSponsoredSlots reads
-- the active rows, best-bid first.
CREATE TABLE IF NOT EXISTS ad_campaigns (
    id          TEXT PRIMARY KEY,
    seller_id   TEXT NOT NULL DEFAULT '',    -- bound from the authenticated seller
    listing_id  TEXT NOT NULL,               -- listing to render in sponsored slots
    budget      BIGINT NOT NULL DEFAULT 0,   -- total budget in minor units; display only
    bid         BIGINT NOT NULL DEFAULT 0,   -- per-slot bid in minor units; ranks slots
    status      INT NOT NULL DEFAULT 1,      -- AdCampaignStatus enum (1 active, 2 paused, 3 ended)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ad_campaigns_seller_idx ON ad_campaigns (seller_id);
-- Bid-ordered active-slot scan: highest bid wins the slot.
CREATE INDEX IF NOT EXISTS ad_campaigns_status_bid_idx ON ad_campaigns (status, bid DESC);
