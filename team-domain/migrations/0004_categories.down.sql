-- 0004_categories down — rollback categories table and column.
ALTER TABLE listings DROP COLUMN IF EXISTS category_id;
DROP TABLE IF EXISTS categories;
