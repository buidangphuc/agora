CREATE TABLE IF NOT EXISTS listing_sales (
    listing_id TEXT PRIMARY KEY,
    sold_count BIGINT NOT NULL DEFAULT 0 CONSTRAINT listing_sales_sold_count_nonneg CHECK (sold_count >= 0)
);

CREATE TABLE IF NOT EXISTS listing_sales_orders (
    order_id   TEXT NOT NULL,
    listing_id TEXT NOT NULL,
    quantity   INT  NOT NULL,
    state      TEXT NOT NULL CONSTRAINT listing_sales_orders_state_check CHECK (state IN ('counted', 'reversed')),
    PRIMARY KEY (order_id, listing_id)
);

CREATE INDEX IF NOT EXISTS listing_sales_orders_listing_idx ON listing_sales_orders (listing_id);

CREATE OR REPLACE FUNCTION pg_temp.pb_varint(v bigint) RETURNS bytea LANGUAGE plpgsql IMMUTABLE AS $f$
DECLARE
    result bytea := ''::bytea;
    x bigint := v;
    b int;
BEGIN
    LOOP
        b := (x & 127)::int;
        x := x >> 7;
        IF x = 0 THEN
            RETURN result || set_byte('\x00'::bytea, 0, b);
        END IF;
        result := result || set_byte('\x00'::bytea, 0, b | 128);
    END LOOP;
END
$f$;

CREATE OR REPLACE FUNCTION pg_temp.pb_len(tag int, body bytea) RETURNS bytea LANGUAGE sql IMMUTABLE AS $f$
    SELECT set_byte('\x00'::bytea, 0, tag) || pg_temp.pb_varint(octet_length(body)) || body
$f$;

CREATE OR REPLACE FUNCTION pg_temp.pb_timestamp(ts timestamptz) RETURNS bytea LANGUAGE sql IMMUTABLE AS $f$
    SELECT '\x08'::bytea || pg_temp.pb_varint(floor(extract(epoch FROM ts))::bigint)
        || CASE WHEN extract(microseconds FROM ts)::bigint % 1000000 = 0 THEN ''::bytea
                ELSE '\x10'::bytea || pg_temp.pb_varint((extract(microseconds FROM ts)::bigint % 1000000) * 1000) END
$f$;

INSERT INTO listing_sales_orders (order_id, listing_id, quantity, state)
SELECT o.id, i.listing_id, SUM(i.quantity)::int,
       CASE
           WHEN o.status = 5 OR EXISTS (SELECT 1 FROM order_returns r WHERE r.order_id = o.id AND r.status = 4)
               THEN 'reversed'
           ELSE 'counted'
       END
FROM orders o
JOIN order_items i ON i.order_id = o.id
WHERE o.paid_at IS NOT NULL
   OR (o.paid_at IS NULL AND o.status IN (2, 3, 4) AND o.payment_method <> 1)
GROUP BY o.id, i.listing_id, o.status
ON CONFLICT (order_id, listing_id) DO NOTHING;

INSERT INTO listing_sales (listing_id, sold_count)
SELECT li.listing_id,
       COALESCE((SELECT SUM(s.quantity) FROM listing_sales_orders s WHERE s.listing_id = li.listing_id AND s.state = 'counted'), 0)
FROM (SELECT DISTINCT listing_id FROM order_items) li
ON CONFLICT (listing_id) DO UPDATE SET sold_count = EXCLUDED.sold_count;

INSERT INTO order_outbox_events (event_id, aggregate_type, aggregate_id, event_type, payload)
SELECT e.event_id, 'Listing', e.listing_id, 'platform.order.v1.ListingSalesChanged',
       pg_temp.pb_len(10, convert_to(e.event_id, 'UTF8'))
    || pg_temp.pb_len(18, convert_to('platform.order.v1.ListingSalesChanged', 'UTF8'))
    || pg_temp.pb_len(26, pg_temp.pb_timestamp(now()))
    || pg_temp.pb_len(58,
           pg_temp.pb_len(10, convert_to(e.listing_id, 'UTF8'))
        || CASE WHEN e.sold_count > 0 THEN '\x10'::bytea || pg_temp.pb_varint(e.sold_count) ELSE ''::bytea END
        || pg_temp.pb_len(26, pg_temp.pb_timestamp(now())))
FROM (
    SELECT listing_id, sold_count,
           md5('listing-sales:' || listing_id || ':' || sold_count::text || ':backfill')::uuid::text AS event_id
    FROM listing_sales
) e
ON CONFLICT (event_id) DO NOTHING;

DROP FUNCTION pg_temp.pb_timestamp(timestamptz);
DROP FUNCTION pg_temp.pb_len(int, bytea);
DROP FUNCTION pg_temp.pb_varint(bigint);
