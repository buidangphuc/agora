-- 0003_images down — drop image_keys column.
ALTER TABLE listings DROP COLUMN IF EXISTS image_keys;
