-- Follow-up to migration 0010: validate its NOT VALID constraints.
--
-- Run MANUALLY, as a separate step after 0010 is deployed:
--   psql "$DATABASE_URL" -f scripts/validate_0010_constraints.sql
--
-- Step 1 prints the violation counts. The script aborts (ON_ERROR_STOP) unless
-- every count is 0, so VALIDATE never runs against violating data: fix the
-- offending rows first. VALIDATE CONSTRAINT takes only a SHARE UPDATE EXCLUSIVE
-- lock (reads and writes continue), so it is safe to run online.

\set ON_ERROR_STOP on

-- Step 1: violation counts.
SELECT 'listings.stock < 0'          AS check_name, count(*) AS violations FROM listings         WHERE stock < 0
UNION ALL
SELECT 'listing_variants.stock < 0',              count(*)               FROM listing_variants WHERE stock < 0
UNION ALL
SELECT 'reservations.status invalid',             count(*)               FROM reservations     WHERE status NOT IN ('active', 'committed', 'released');

-- Abort unless every count is zero.
DO $$
DECLARE n bigint;
BEGIN
    SELECT (SELECT count(*) FROM listings         WHERE stock < 0)
         + (SELECT count(*) FROM listing_variants WHERE stock < 0)
         + (SELECT count(*) FROM reservations     WHERE status NOT IN ('active', 'committed', 'released'))
      INTO n;
    IF n > 0 THEN
        RAISE EXCEPTION 'refusing to VALIDATE: % violating row(s); fix the data first', n;
    END IF;
END
$$;

-- Step 2: validate.
ALTER TABLE listings         VALIDATE CONSTRAINT listings_stock_nonneg;
ALTER TABLE listing_variants VALIDATE CONSTRAINT listing_variants_stock_nonneg;
ALTER TABLE reservations     VALIDATE CONSTRAINT reservations_status_check;

-- Step 3: confirm (convalidated must be true for all three).
SELECT conrelid::regclass AS table_name, conname, convalidated
FROM pg_constraint
WHERE conname IN ('listings_stock_nonneg', 'listing_variants_stock_nonneg', 'reservations_status_check')
ORDER BY conname;
