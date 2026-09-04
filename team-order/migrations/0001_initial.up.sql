-- 0001_initial — Cart and Orders tables for team-order.

CREATE TABLE IF NOT EXISTS cart_items (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL,
    listing_id   TEXT NOT NULL,
    variant_id   TEXT NOT NULL DEFAULT '',
    quantity     INT NOT NULL DEFAULT 1,
    unit_price   BIGINT NOT NULL,
    title        TEXT NOT NULL,
    variant_name TEXT NOT NULL DEFAULT '',
    image_url    TEXT NOT NULL DEFAULT '',
    seller_id    TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT cart_items_unique_user_item UNIQUE (user_id, listing_id, variant_id)
);

CREATE INDEX IF NOT EXISTS cart_items_user_idx ON cart_items (user_id);

CREATE TABLE IF NOT EXISTS orders (
    id               TEXT PRIMARY KEY,
    buyer_id         TEXT NOT NULL,
    seller_id        TEXT NOT NULL,
    status           INT NOT NULL DEFAULT 1,
    total_amount     BIGINT NOT NULL,
    currency         TEXT NOT NULL DEFAULT 'VND',
    shipping_address JSONB NOT NULL DEFAULT '{}'::jsonb,
    tracking_number  TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS orders_buyer_idx ON orders (buyer_id);
CREATE INDEX IF NOT EXISTS orders_seller_idx ON orders (seller_id);

CREATE TABLE IF NOT EXISTS order_items (
    id           TEXT PRIMARY KEY,
    order_id     TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    listing_id   TEXT NOT NULL,
    variant_id   TEXT NOT NULL DEFAULT '',
    title        TEXT NOT NULL,
    variant_name TEXT NOT NULL DEFAULT '',
    quantity     INT NOT NULL,
    unit_price   BIGINT NOT NULL,
    image_url    TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS order_items_order_idx ON order_items (order_id);
