SELECT listing_id AS entity_id,
  seller_id AS seller_id,
  category_id AS category_id,
  price::BIGINT AS price,
  status AS status
FROM listings
WHERE listing_id IS NOT NULL AND listing_id <> '' AND updated_at <= $as_of
