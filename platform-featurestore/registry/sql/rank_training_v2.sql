-- rank_training@v2, per (impression_id, listing_id) of an `impression` event in the window. Reads only the point-in-time
-- table `events` and the parameters $as_of and $window_days. label: 2 when an add_to_cart carrying the same impression_id
-- and listing_id happened at or after the impression, else 1 for a click, else 0. The earliest impression event of a
-- pair is the row. Impressions without an impression_id, a listing_id or a user_key are not rows.
WITH -- Consent (recsys-consent-and-context): an event counts only when consent_personalization is true and
-- consent_version > 0 (null/0 = denied), and only when it is after that user's latest personalization=false event.
cons AS (
  SELECT e.*
  FROM events e
  LEFT JOIN (
    SELECT user_key, max(occurred_at) AS denied_at FROM events
    WHERE consent_personalization = false GROUP BY user_key
  ) b ON b.user_key = e.user_key
  WHERE e.consent_personalization IS TRUE AND coalesce(e.consent_version, 0) > 0
    AND (b.denied_at IS NULL OR e.occurred_at > b.denied_at)
),
imp AS (
  SELECT impression_id, listing_id, user_key, coalesce(position, 0)::BIGINT AS position, occurred_at,
    row_number() OVER (PARTITION BY impression_id, listing_id ORDER BY occurred_at, event_id) AS rn
  FROM cons
  WHERE event_type = 'impression'
    AND impression_id IS NOT NULL AND impression_id <> ''
    AND listing_id IS NOT NULL AND listing_id <> ''
    AND user_key IS NOT NULL AND user_key <> ''
    AND occurred_at > $as_of - to_days($window_days::INTEGER) AND occurred_at <= $as_of
)
SELECT i.user_key, i.impression_id, i.listing_id, i.position,
  coalesce(max(CASE a.event_type WHEN 'add_to_cart' THEN 2 WHEN 'click' THEN 1 END), 0)::BIGINT AS label,
  i.occurred_at
FROM imp i
LEFT JOIN cons a
  ON a.event_type IN ('click', 'add_to_cart')
 AND a.impression_id = i.impression_id AND a.listing_id = i.listing_id
 AND a.occurred_at >= i.occurred_at AND a.occurred_at <= $as_of
WHERE i.rn = 1
GROUP BY i.user_key, i.impression_id, i.listing_id, i.position, i.occurred_at
ORDER BY i.occurred_at, i.impression_id, i.listing_id
