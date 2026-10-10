-- user_activity@v2, per user_key. Reads only the point-in-time views (events, facts, orders) and $as_of.
-- v2 adds paid_orders_30d: distinct paid orders of the buyer in (as_of - 30d, as_of]; lines without a buyer count for nobody.
WITH ev AS (
  SELECT user_key,
    count(*) FILTER (WHERE event_type = 'view') AS views_7d,
    count(*) FILTER (WHERE event_type = 'click') AS clicks_7d,
    count(*) FILTER (WHERE event_type = 'add_to_cart') AS add_to_cart_7d
  FROM events
  WHERE user_key IS NOT NULL
    AND occurred_at > $as_of - INTERVAL 7 DAY AND occurred_at <= $as_of
  GROUP BY user_key
),
seen AS (
  SELECT DISTINCT user_key FROM events WHERE user_key IS NOT NULL AND occurred_at <= $as_of
),
latest AS (
  SELECT user_id, fact, listing_id, seller_id,
    row_number() OVER (
      PARTITION BY user_id, CASE WHEN fact IN ('favorite_added', 'favorite_removed') THEN 'f:' || listing_id
                                 ELSE 's:' || seller_id END
      ORDER BY occurred_at DESC, event_id DESC) AS rn
  FROM facts
  WHERE occurred_at <= $as_of
    AND fact IN ('favorite_added', 'favorite_removed', 'seller_followed', 'seller_unfollowed')
    AND user_id IS NOT NULL
),
cur AS (
  SELECT user_id,
    count(*) FILTER (WHERE fact = 'favorite_added') AS favorites_current,
    count(*) FILTER (WHERE fact = 'seller_followed') AS follows_current
  FROM latest WHERE rn = 1
  GROUP BY user_id
),
ord AS (
  SELECT buyer_id AS user_id, count(DISTINCT order_id) AS paid_orders_30d
  FROM orders
  WHERE buyer_id IS NOT NULL AND buyer_id <> '' AND status = 'PAID'
    AND occurred_at > $as_of - INTERVAL 30 DAY AND occurred_at <= $as_of
  GROUP BY buyer_id
),
keys AS (
  SELECT user_key AS entity_id FROM seen
  UNION
  SELECT user_id FROM cur
  UNION
  SELECT user_id FROM ord
)
SELECT k.entity_id,
  coalesce(ev.views_7d, 0)::BIGINT AS views_7d,
  coalesce(ev.clicks_7d, 0)::BIGINT AS clicks_7d,
  coalesce(ev.add_to_cart_7d, 0)::BIGINT AS add_to_cart_7d,
  coalesce(cur.favorites_current, 0)::BIGINT AS favorites_current,
  coalesce(cur.follows_current, 0)::BIGINT AS follows_current,
  coalesce(ord.paid_orders_30d, 0)::BIGINT AS paid_orders_30d
FROM keys k
LEFT JOIN ev ON ev.user_key = k.entity_id
LEFT JOIN cur ON cur.user_id = k.entity_id
LEFT JOIN ord ON ord.user_id = k.entity_id
