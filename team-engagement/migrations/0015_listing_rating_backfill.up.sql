CREATE OR REPLACE FUNCTION pg_temp.pb_varint(n bigint) RETURNS bytea LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE
    out bytea := ''::bytea;
    v   bigint := n;
BEGIN
    WHILE v >= 128 LOOP
        out := out || set_byte('\x00'::bytea, 0, (v % 128)::int + 128);
        v := v / 128;
    END LOOP;
    RETURN out || set_byte('\x00'::bytea, 0, v::int);
END $$;

CREATE OR REPLACE FUNCTION pg_temp.pb_bytes(field int, b bytea) RETURNS bytea LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE WHEN length(b) = 0 THEN ''::bytea
                ELSE set_byte('\x00'::bytea, 0, field * 8 + 2) || pg_temp.pb_varint(length(b)) || b END
$$;

CREATE OR REPLACE FUNCTION pg_temp.pb_msg(field int, b bytea) RETURNS bytea LANGUAGE sql IMMUTABLE AS $$
    SELECT set_byte('\x00'::bytea, 0, field * 8 + 2) || pg_temp.pb_varint(length(b)) || b
$$;

CREATE OR REPLACE FUNCTION pg_temp.pb_int(field int, n bigint) RETURNS bytea LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE WHEN n = 0 THEN ''::bytea
                ELSE set_byte('\x00'::bytea, 0, field * 8) || pg_temp.pb_varint(n) END
$$;

CREATE OR REPLACE FUNCTION pg_temp.pb_double(field int, d float8) RETURNS bytea LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE
    be  bytea := float8send(d);
    le  bytea := ''::bytea;
    i   int;
BEGIN
    IF d = 0 THEN
        RETURN ''::bytea;
    END IF;
    FOR i IN REVERSE 7..0 LOOP
        le := le || set_byte('\x00'::bytea, 0, get_byte(be, i));
    END LOOP;
    RETURN set_byte('\x00'::bytea, 0, field * 8 + 1) || le;
END $$;

CREATE OR REPLACE FUNCTION pg_temp.pb_ts(secs bigint, nanos bigint) RETURNS bytea LANGUAGE sql IMMUTABLE AS $$
    SELECT pg_temp.pb_int(1, secs) || pg_temp.pb_int(2, nanos)
$$;

INSERT INTO engagement_outbox_events
    (event_id, aggregate_type, aggregate_id, event_type, payload, request_id,
     status, attempts, available_at, created_at)
SELECT s.event_id,
       'Listing',
       s.listing_id,
       'platform.engagement.v1.ListingRatingChanged',
       pg_temp.pb_bytes(1, convert_to(s.event_id, 'UTF8'))
       || pg_temp.pb_bytes(2, convert_to('platform.engagement.v1.ListingRatingChanged', 'UTF8'))
       || pg_temp.pb_msg(3, pg_temp.pb_ts(s.secs, s.nanos))
       || pg_temp.pb_bytes(7,
              pg_temp.pb_bytes(1, convert_to(s.listing_id, 'UTF8'))
              || pg_temp.pb_double(2, s.avg_rating)
              || pg_temp.pb_int(3, s.review_count)
              || pg_temp.pb_msg(4, pg_temp.pb_ts(s.secs, s.nanos))),
       '',
       'pending',
       0,
       s.ts,
       s.ts
FROM (
    SELECT md5('listing-rating-backfill:' || r.listing_id)::uuid::text AS event_id,
           r.listing_id,
           count(*) FILTER (WHERE r.verified_purchase AND r.hidden_at IS NULL) AS review_count,
           COALESCE((sum(r.rating) FILTER (WHERE r.verified_purchase AND r.hidden_at IS NULL))::float8 / NULLIF(count(*) FILTER (WHERE r.verified_purchase AND r.hidden_at IS NULL), 0), 0) AS avg_rating,
           t.ts,
           floor(extract(epoch FROM t.ts))::bigint AS secs,
           (extract(microseconds FROM t.ts)::bigint % 1000000) * 1000 AS nanos
    FROM reviews r
    CROSS JOIN (SELECT now() AS ts) t
    GROUP BY r.listing_id, t.ts
) s
ORDER BY s.listing_id
ON CONFLICT (event_id) DO NOTHING;

DROP FUNCTION pg_temp.pb_ts(bigint, bigint);
DROP FUNCTION pg_temp.pb_double(int, float8);
DROP FUNCTION pg_temp.pb_int(int, bigint);
DROP FUNCTION pg_temp.pb_msg(int, bytea);
DROP FUNCTION pg_temp.pb_bytes(int, bytea);
DROP FUNCTION pg_temp.pb_varint(bigint);
