ALTER TABLE users
    DROP COLUMN IF EXISTS seller_terms_accepted_at,
    DROP COLUMN IF EXISTS seller_terms_version;
