-- Attribute a link to the authenticated principal that created it ('' = anonymous).
ALTER TABLE share_links ADD COLUMN IF NOT EXISTS created_by TEXT NOT NULL DEFAULT '';
