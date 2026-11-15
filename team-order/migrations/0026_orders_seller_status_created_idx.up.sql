CREATE INDEX CONCURRENTLY IF NOT EXISTS orders_seller_status_created_idx ON orders (seller_id, status, created_at DESC, id DESC);
