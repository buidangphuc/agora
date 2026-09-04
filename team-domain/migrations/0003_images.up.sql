-- 0003_images — Storage keys for product images (Phase 3).
-- DB stores array of storage keys only, no raw blob.
ALTER TABLE listings ADD COLUMN IF NOT EXISTS image_keys TEXT[] NOT NULL DEFAULT '{}';
