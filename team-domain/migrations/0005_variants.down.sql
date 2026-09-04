-- 0005_variants down — rollback listing_variants and stock column.
DROP TABLE IF EXISTS listing_variants;
ALTER TABLE listings DROP COLUMN IF EXISTS stock;
