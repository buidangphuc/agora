-- user_preferences@v1, per user_key. Up to three categories, ranked by the weighted count of the user's views (1),
-- clicks (2) and add-to-carts (5) in (as_of - 30d, as_of] on listings with a known category, weight descending then
-- name, comma-joined. A category id containing a comma is skipped. Reads events, listings and $as_of.
WITH w AS (
  SELECT * FROM (VALUES ('view', 1.0), ('click', 2.0), ('add_to_cart', 5.0)) AS t(event_type, weight)
),
scored AS (
  SELECT e.user_key, l.category_id, sum(w.weight) AS score
  FROM events e
  JOIN w ON w.event_type = e.event_type
  JOIN listings l ON l.listing_id = e.listing_id
  WHERE e.user_key IS NOT NULL AND e.user_key <> ''
    AND l.category_id IS NOT NULL AND l.category_id <> '' AND strpos(l.category_id, ',') = 0
    AND e.occurred_at > $as_of - INTERVAL 30 DAY AND e.occurred_at <= $as_of
  GROUP BY e.user_key, l.category_id
),
ranked AS (
  SELECT user_key, category_id,
    row_number() OVER (PARTITION BY user_key ORDER BY score DESC, category_id) AS rn
  FROM scored
)
SELECT user_key AS entity_id,
  string_agg(category_id, ',' ORDER BY rn) AS preferred_categories
FROM ranked
WHERE rn <= 3
GROUP BY user_key
