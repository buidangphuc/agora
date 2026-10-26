-- 0001_promotion — Voucher, redemption reservation, and flash-sale tables for
-- team-promotion. Discounts are pure calculation; no money movement (AGENTS.md §7).

CREATE TABLE IF NOT EXISTS vouchers (
    id             TEXT PRIMARY KEY,
    code           TEXT NOT NULL,
    scope          INT NOT NULL DEFAULT 0,      -- VoucherScope enum (0 unspecified, 1 shop, 2 platform)
    seller_id      TEXT NOT NULL DEFAULT '',    -- set for shop-scoped vouchers
    discount_type  INT NOT NULL DEFAULT 0,      -- DiscountType enum (1 percent, 2 fixed)
    discount_value BIGINT NOT NULL DEFAULT 0,   -- percent (1-100) or minor-unit amount
    min_spend      BIGINT NOT NULL DEFAULT 0,
    max_discount   BIGINT NOT NULL DEFAULT 0,   -- cap for percent discounts; 0 = uncapped
    quota          BIGINT NOT NULL DEFAULT 0,   -- total redemptions allowed; 0 = unlimited
    used           BIGINT NOT NULL DEFAULT 0,
    starts_at      TIMESTAMPTZ,
    ends_at        TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT vouchers_code_unique UNIQUE (code)
);

CREATE INDEX IF NOT EXISTS vouchers_seller_idx ON vouchers (seller_id);

-- voucher_reservations backs the idempotent redemption hold: ValidateAndReserve
-- creates a row keyed on the caller's reservation_id (order saga id), Commit
-- finalizes it, Release cancels it. UNIQUE(reservation_id) enforces idempotency
-- so a retried reserve never decrements quota twice (mirrors ADR-0008).
CREATE TABLE IF NOT EXISTS voucher_reservations (
    id              TEXT PRIMARY KEY,
    reservation_id  TEXT NOT NULL,
    voucher_id      TEXT NOT NULL REFERENCES vouchers(id) ON DELETE CASCADE,
    buyer_id        TEXT NOT NULL,
    discount_amount BIGINT NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'reserved',  -- reserved | committed | released
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT voucher_reservations_reservation_unique UNIQUE (reservation_id)
);

CREATE INDEX IF NOT EXISTS voucher_reservations_voucher_idx ON voucher_reservations (voucher_id);

CREATE TABLE IF NOT EXISTS flash_sale_campaigns (
    id          TEXT PRIMARY KEY,
    listing_id  TEXT NOT NULL,
    variant_id  TEXT NOT NULL DEFAULT '',
    sale_price  BIGINT NOT NULL,
    stock_cap   BIGINT NOT NULL DEFAULT 0,
    stock_sold  BIGINT NOT NULL DEFAULT 0,
    starts_at   TIMESTAMPTZ,
    ends_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS flash_sale_campaigns_listing_idx ON flash_sale_campaigns (listing_id);
