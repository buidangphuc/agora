-- 0010_reservation_lifecycle_constraints — `committed` reservation status and
-- stock >= 0 backstops (OpenSpec change port-order-inventory-correctness, D2).
--
-- A reservation moves active -> committed | released, and committed -> released.
-- CommitReservation (called by team-order before it places an order) sets
-- `committed`; the TTL sweeper only restores status = 'active', so a placed
-- order's stock is never swept back. Stock is restored once, when a reservation
-- becomes `released`.
--
-- All constraints are added NOT VALID: every new write is checked immediately,
-- but existing rows are not scanned, so pre-existing odd data cannot block the
-- deploy. Validate them as a separate step with
-- scripts/validate_0010_constraints.sql (it prints violation counts and refuses to
-- VALIDATE unless every count is zero). Do NOT validate inside this migration.
--
-- Guarded by pg_constraint lookups so a re-run is a no-op (ADD CONSTRAINT has no
-- IF NOT EXISTS).

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'reservations_status_check') THEN
        ALTER TABLE reservations
            ADD CONSTRAINT reservations_status_check
            CHECK (status IN ('active', 'committed', 'released')) NOT VALID;
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'listings_stock_nonneg') THEN
        ALTER TABLE listings
            ADD CONSTRAINT listings_stock_nonneg
            CHECK (stock >= 0) NOT VALID;
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'listing_variants_stock_nonneg') THEN
        ALTER TABLE listing_variants
            ADD CONSTRAINT listing_variants_stock_nonneg
            CHECK (stock >= 0) NOT VALID;
    END IF;
END
$$;

COMMENT ON COLUMN reservations.status IS 'active | committed | released (active -> committed | released; committed -> released)';
