-- 0002_subscriptions — Seller subscription tiers (shop upgrade plans) for
-- team-promotion. Entitlements are pure calculation; subscribing is MOCK and moves
-- no money (payments/wallet stay mock, AGENTS.md §7).

-- subscription_plans is read-only reference data. Rows are seeded below and kept in
-- sync with repository.DefaultPlans() (the single source of truth for both the
-- Postgres and in-memory backends).
CREATE TABLE IF NOT EXISTS subscription_plans (
    id         TEXT PRIMARY KEY,
    tier       INT NOT NULL,                  -- PlanTier enum (1 free, 2 pro, 3 premium)
    price      BIGINT NOT NULL DEFAULT 0,     -- minor units per billing period; display only
    features   TEXT[] NOT NULL DEFAULT '{}',  -- human-readable bullets (Vietnamese copy)
    limits     JSONB NOT NULL DEFAULT '{}',   -- entitlement limits keyed by feature
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- seller_subscriptions holds at most one row per seller (UNIQUE seller_id): taking a
-- new plan upserts. status is 'active' | 'expired' | 'cancelled'.
CREATE TABLE IF NOT EXISTS seller_subscriptions (
    id         TEXT PRIMARY KEY,
    seller_id  TEXT NOT NULL,
    plan_id    TEXT NOT NULL REFERENCES subscription_plans(id),
    status     TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT seller_subscriptions_seller_unique UNIQUE (seller_id)
);

CREATE INDEX IF NOT EXISTS seller_subscriptions_seller_idx ON seller_subscriptions (seller_id);

-- Seed FREE / PRO / PREMIUM (idempotent). Prices are display-only minor units.
INSERT INTO subscription_plans (id, tier, price, features, limits) VALUES
    ('plan_free', 1, 0,
     ARRAY['Đăng tối đa 10 tin', 'Thống kê cơ bản'],
     '{"max_listings":"10","featured_listings":"0","analytics":"basic","priority_support":"false"}'),
    ('plan_pro', 2, 19900000,
     ARRAY['Đăng tối đa 100 tin', '5 tin nổi bật mỗi tháng', 'Thống kê nâng cao'],
     '{"max_listings":"100","featured_listings":"5","analytics":"advanced","priority_support":"false"}'),
    ('plan_premium', 3, 49900000,
     ARRAY['Đăng tin không giới hạn', '50 tin nổi bật mỗi tháng', 'Thống kê nâng cao', 'Hỗ trợ ưu tiên'],
     '{"max_listings":"-1","featured_listings":"50","analytics":"advanced","priority_support":"true"}')
ON CONFLICT (id) DO NOTHING;
