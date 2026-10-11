-- 0010_reservation_lifecycle_constraints (down) — drop the added CHECK constraints.
ALTER TABLE reservations     DROP CONSTRAINT IF EXISTS reservations_status_check;
ALTER TABLE listings         DROP CONSTRAINT IF EXISTS listings_stock_nonneg;
ALTER TABLE listing_variants DROP CONSTRAINT IF EXISTS listing_variants_stock_nonneg;
COMMENT ON COLUMN reservations.status IS NULL;
