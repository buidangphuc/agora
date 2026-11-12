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

CREATE TEMP TABLE pg_temp.cod_backfill_added (listing_id TEXT PRIMARY KEY, quantity BIGINT NOT NULL);

WITH ins AS (
    INSERT INTO listing_sales_orders (order_id, listing_id, quantity, state)
    SELECT o.id, i.listing_id, SUM(i.quantity)::int, 'counted'
    FROM orders o
    JOIN order_items i ON i.order_id = o.id
    WHERE o.status = 4 AND o.payment_method = 1
    GROUP BY o.id, i.listing_id
    ON CONFLICT (order_id, listing_id) DO NOTHING
    RETURNING listing_id, quantity
)
INSERT INTO pg_temp.cod_backfill_added (listing_id, quantity)
SELECT listing_id, SUM(quantity) FROM ins GROUP BY listing_id;

INSERT INTO listing_sales (listing_id, sold_count)
SELECT listing_id, quantity FROM pg_temp.cod_backfill_added
ON CONFLICT (listing_id) DO UPDATE SET sold_count = listing_sales.sold_count + EXCLUDED.sold_count;

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
    SELECT s.listing_id, s.sold_count,
           md5('listing-sales:' || s.listing_id || ':' || s.sold_count::text || ':0024-backfill-cod')::uuid::text AS event_id
    FROM listing_sales s
    JOIN pg_temp.cod_backfill_added a ON a.listing_id = s.listing_id
) e
ON CONFLICT (event_id) DO NOTHING;

DROP TABLE pg_temp.cod_backfill_added;
DROP FUNCTION pg_temp.pb_timestamp(timestamptz);
DROP FUNCTION pg_temp.pb_len(int, bytea);
DROP FUNCTION pg_temp.pb_varint(bigint);
