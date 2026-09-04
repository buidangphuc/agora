-- 0003_returns_and_shipments — RMA (order returns) and Shipment tracking tables

CREATE TABLE IF NOT EXISTS order_returns (
    id            TEXT PRIMARY KEY,
    order_id      TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    buyer_id      TEXT NOT NULL,
    seller_id     TEXT NOT NULL,
    reason        TEXT NOT NULL,
    refund_amount BIGINT NOT NULL,
    status        INT NOT NULL DEFAULT 1, -- 1: PENDING, 2: APPROVED, 3: REJECTED, 4: REFUNDED
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS order_returns_order_idx ON order_returns (order_id);
CREATE INDEX IF NOT EXISTS order_returns_buyer_idx ON order_returns (buyer_id);
CREATE INDEX IF NOT EXISTS order_returns_seller_idx ON order_returns (seller_id);

CREATE TABLE IF NOT EXISTS shipments (
    id            TEXT PRIMARY KEY,
    order_id      TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    carrier       TEXT NOT NULL, -- SPX, GHN, GHTK, etc.
    tracking_code TEXT NOT NULL UNIQUE,
    status        INT NOT NULL DEFAULT 1, -- 1: PENDING, 2: PICKED_UP, 3: IN_TRANSIT, 4: DELIVERED, 5: FAILED
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS shipments_order_idx ON shipments (order_id);
CREATE INDEX IF NOT EXISTS shipments_tracking_code_idx ON shipments (tracking_code);

CREATE TABLE IF NOT EXISTS shipment_checkpoints (
    id          TEXT PRIMARY KEY,
    shipment_id TEXT NOT NULL REFERENCES shipments(id) ON DELETE CASCADE,
    timestamp   TIMESTAMPTZ NOT NULL DEFAULT now(),
    location    TEXT NOT NULL,
    description TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS shipment_checkpoints_shipment_idx ON shipment_checkpoints (shipment_id);
