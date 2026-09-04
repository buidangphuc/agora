-- 0004_saga_and_processed_events — durable purchase saga (AD3/AD5/M6/M7) and
-- idempotent event consumption (AD4/H3).
--
-- order_reservations persists the stock-reservation intent BEFORE the external
-- ReserveStock call so a crash between reserve and order-persist leaves a durable
-- row the TTL sweep can release (fixes SA-C2, the in-memory best-effort saga that
-- leaked stock). Each row is keyed by a stable reservation_id per (cart_item,
-- attempt) so retries are idempotent (AD5/M6). A reservation is COMMITTED once its
-- owning seller-order is persisted; compensation never releases COMMITTED rows so
-- a multi-seller partial failure cannot orphan an already-persisted order's stock
-- (M7).

CREATE TABLE IF NOT EXISTS order_sagas (
    id          TEXT PRIMARY KEY,
    buyer_id    TEXT NOT NULL,
    status      INT  NOT NULL DEFAULT 1, -- 1: PENDING, 2: COMPLETED, 3: COMPENSATED, 4: FAILED
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS order_sagas_buyer_idx ON order_sagas (buyer_id);

CREATE TABLE IF NOT EXISTS order_reservations (
    id          TEXT PRIMARY KEY,        -- stable reservation_id per (cart_item, attempt)
    saga_id     TEXT NOT NULL,
    order_id    TEXT,                    -- set when the owning seller-order is persisted (COMMITTED)
    seller_id   TEXT NOT NULL DEFAULT '',
    buyer_id    TEXT NOT NULL DEFAULT '',
    listing_id  TEXT NOT NULL,
    variant_id  TEXT NOT NULL DEFAULT '',
    quantity    INT  NOT NULL,
    -- 1: PENDING (intent persisted, not yet reserved), 2: RESERVED (stock decremented),
    -- 3: COMMITTED (owning order persisted; never released), 4: RELEASED (compensated),
    -- 5: RELEASE_FAILED (parked for the TTL sweep to retry), 6: FAILED (reserve rejected)
    status      INT  NOT NULL DEFAULT 1,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS order_reservations_saga_idx    ON order_reservations (saga_id);
CREATE INDEX IF NOT EXISTS order_reservations_order_idx   ON order_reservations (order_id);
CREATE INDEX IF NOT EXISTS order_reservations_sweep_idx   ON order_reservations (status, expires_at);

-- processed_events is the consumer dedupe ledger (AD4): the event_id of every
-- PaymentSettled we have already applied. Consuming the same event twice is a
-- no-op because the second insert conflicts on the primary key (H3).
CREATE TABLE IF NOT EXISTS processed_events (
    event_id     TEXT PRIMARY KEY,
    consumer     TEXT NOT NULL DEFAULT '',
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
