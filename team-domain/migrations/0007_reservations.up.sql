-- 0007_reservations — idempotent stock reservations (AD5) + TTL sweep (AD3).
--
-- A reservation row is written IN THE SAME TRANSACTION as the stock decrement,
-- keyed by a caller-supplied reservation_id (stable per cart-item + attempt).
-- A repeat reserve with the same id finds the row and is a no-op, so a retried
-- checkout never double-decrements (SA-M6 / AD5).
--
-- Every reservation carries an expires_at TTL. The domain-side sweeper
-- (internal/service/sweeper.go) releases still-active reservations past their
-- expires_at and restores the reserved stock, so a checkout that crashes before
-- confirming can never leak stock (SA-C2 / AD3). The sweep is idempotent: a
-- released row is skipped on the next pass.

CREATE TABLE IF NOT EXISTS reservations (
    reservation_id TEXT        PRIMARY KEY,               -- stable per (cart_item, attempt); the idempotency key
    listing_id     TEXT        NOT NULL,                  -- listing whose stock was decremented
    variant_id     TEXT        NOT NULL DEFAULT '',       -- '' => base stock; else the variant
    quantity       INT         NOT NULL,                  -- units reserved (restored on sweep)
    status         TEXT        NOT NULL DEFAULT 'active', -- active | released
    expires_at     TIMESTAMPTZ NOT NULL,                  -- TTL: swept once now() passes this
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    released_at    TIMESTAMPTZ                            -- set when the sweeper restores the stock
);

-- Sweep support: only still-active reservations are scanned, ordered by TTL.
CREATE INDEX IF NOT EXISTS reservations_sweep_idx
    ON reservations (expires_at)
    WHERE status = 'active';
