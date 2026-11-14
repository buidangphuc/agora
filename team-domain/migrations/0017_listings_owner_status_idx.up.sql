CREATE INDEX IF NOT EXISTS listings_owner_status_idx ON listings (seller_id, status, id);
