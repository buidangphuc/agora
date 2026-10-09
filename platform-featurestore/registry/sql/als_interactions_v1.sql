-- als_interactions@v1, per (user_key, listing_id). Reads only the point-in-time tables (events, facts) and the
-- parameters $as_of and $window_days. Weights per signal: see the VALUES table and the favourite/review terms below.
WITH w AS (
  SELECT * FROM (VALUES
    ('impression', 0.5), ('view', 1.0), ('click', 2.0), ('view_cart', 2.5), ('add_to_cart', 5.0),
    ('add_shipping_info', 6.0), ('add_payment_info', 7.0), ('begin_checkout', 8.0), ('purchase', 10.0)
  ) AS t(event_type, weight)
),
ev AS (
  SELECT e.user_key, e.listing_id,
    sum(coalesce(w.weight, 0.0)) AS weight,
    count(*) AS interactions,
    max(e.occurred_at) AS last_at
  FROM events e LEFT JOIN w ON w.event_type = e.event_type
  WHERE e.user_key IS NOT NULL AND e.user_key <> '' AND e.listing_id IS NOT NULL AND e.listing_id <> ''
    AND e.occurred_at > $as_of - to_days($window_days::INTEGER) AND e.occurred_at <= $as_of
  GROUP BY e.user_key, e.listing_id
),
fav_latest AS (
  SELECT user_id AS user_key, listing_id, fact, occurred_at,
    row_number() OVER (PARTITION BY user_id, listing_id ORDER BY occurred_at DESC, event_id DESC) AS rn
  FROM facts
  WHERE fact IN ('favorite_added', 'favorite_removed')
    AND user_id IS NOT NULL AND user_id <> '' AND listing_id IS NOT NULL AND listing_id <> ''
    AND occurred_at > $as_of - to_days($window_days::INTEGER) AND occurred_at <= $as_of
),
fav AS (
  SELECT user_key, listing_id, 3.0 AS weight, 1 AS interactions, occurred_at AS last_at
  FROM fav_latest WHERE rn = 1 AND fact = 'favorite_added'
),
rev_latest AS (
  SELECT user_id AS user_key, listing_id, rating, occurred_at,
    row_number() OVER (PARTITION BY user_id, listing_id ORDER BY occurred_at DESC, event_id DESC) AS rn
  FROM facts
  WHERE fact = 'review_created'
    AND user_id IS NOT NULL AND user_id <> '' AND listing_id IS NOT NULL AND listing_id <> ''
    AND occurred_at > $as_of - to_days($window_days::INTEGER) AND occurred_at <= $as_of
),
rev AS (
  SELECT user_key, listing_id,
    CASE WHEN rating >= 4 THEN 2.0 WHEN rating <= 2 THEN -2.0 ELSE 0.0 END AS weight,
    1 AS interactions, occurred_at AS last_at
  FROM rev_latest WHERE rn = 1
),
allrows AS (
  SELECT * FROM ev
  UNION ALL SELECT * FROM fav
  UNION ALL SELECT * FROM rev
)
SELECT user_key, listing_id,
  sum(weight)::DOUBLE AS weight,
  sum(interactions)::BIGINT AS interactions,
  max(last_at) AS last_occurred_at
FROM allrows
GROUP BY user_key, listing_id
HAVING sum(weight) > 0
ORDER BY user_key, listing_id
