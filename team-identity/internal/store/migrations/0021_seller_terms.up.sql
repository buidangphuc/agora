ALTER TABLE users
    ADD COLUMN IF NOT EXISTS seller_terms_version TEXT,
    ADD COLUMN IF NOT EXISTS seller_terms_accepted_at TIMESTAMPTZ;
