-- item_attributes@v1, per listing_id. Reads only the point-in-time table `listings` (latest change per listing,
-- updated_at <= $as_of, also enforced when the table is loaded). An attribute that is unknown is NULL.
SELECT listing_id AS entity_id,
  seller_id AS seller_id,
  category_id AS category_id,
  price::BIGINT AS price
FROM listings
WHERE listing_id IS NOT NULL AND listing_id <> '' AND updated_at <= $as_of
