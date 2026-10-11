-- item_popularity@v1, per listing_id. Reads only the point-in-time views (events, facts) and $as_of.
WITH ev AS (
  SELECT listing_id,
    count(*) FILTER (WHERE event_type = 'view') AS views_7d,
    count(*) FILTER (WHERE event_type = 'click') AS clicks_7d,
    count(*) FILTER (WHERE event_type = 'add_to_cart') AS add_to_cart_7d,
    count(*) FILTER (WHERE event_type = 'impression') AS impressions_7d
  FROM events
  WHERE listing_id IS NOT NULL AND listing_id <> ''
    AND occurred_at > $as_of - INTERVAL 7 DAY AND occurred_at <= $as_of
  GROUP BY listing_id
),
latest AS (
  SELECT user_id, listing_id, fact,
    row_number() OVER (PARTITION BY user_id, listing_id ORDER BY occurred_at DESC, event_id DESC) AS rn
  FROM facts
  WHERE occurred_at <= $as_of AND fact IN ('favorite_added', 'favorite_removed')
    AND user_id IS NOT NULL AND listing_id IS NOT NULL AND listing_id <> ''
),
fav AS (
  SELECT listing_id, count(*) AS favorites_current FROM latest WHERE rn = 1 AND fact = 'favorite_added'
  GROUP BY listing_id
),
rev AS (
  SELECT listing_id, count(*) AS review_count, avg(rating)::DOUBLE AS avg_rating
  FROM facts
  WHERE fact = 'review_created' AND occurred_at <= $as_of AND listing_id IS NOT NULL AND listing_id <> ''
  GROUP BY listing_id
),
keys AS (
  SELECT listing_id FROM ev UNION SELECT listing_id FROM fav UNION SELECT listing_id FROM rev
)
SELECT k.listing_id AS entity_id,
  coalesce(ev.views_7d, 0)::BIGINT AS views_7d,
  coalesce(ev.clicks_7d, 0)::BIGINT AS clicks_7d,
  coalesce(ev.add_to_cart_7d, 0)::BIGINT AS add_to_cart_7d,
  coalesce(fav.favorites_current, 0)::BIGINT AS favorites_current,
  coalesce(rev.review_count, 0)::BIGINT AS review_count,
  rev.avg_rating AS avg_rating,
  CASE WHEN coalesce(ev.impressions_7d, 0) = 0 THEN 0.0
       ELSE coalesce(ev.clicks_7d, 0)::DOUBLE / ev.impressions_7d END AS ctr_7d
FROM keys k
LEFT JOIN ev ON ev.listing_id = k.listing_id
LEFT JOIN fav ON fav.listing_id = k.listing_id
LEFT JOIN rev ON rev.listing_id = k.listing_id
