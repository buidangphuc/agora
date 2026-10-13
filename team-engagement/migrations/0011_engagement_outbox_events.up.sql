-- 0011_engagement_outbox_events.up.sql: the engagement outbox moves to the go-sdk
-- shared shape (outbox.SchemaSQL("engagement_outbox_events")). Forward-only.
--
-- The old `outbox` table (seq, key/type columns, bare fact payload, envelope built
-- at relay time, advisory-lock relayer) STAYS IN PLACE, untouched, for a later
-- cleanup change to drop. From now on the service writes full EventEnvelopes to
-- the new table and the go-sdk relayer publishes them.
--
-- Rows not yet published when this runs are copied over so no fact is lost:
--   * event_id is kept (consumers dedupe on it);
--   * created_at = occurred_at;
--   * the payload becomes the marshalled platform.events.v1.EventEnvelope, built
--     here in SQL with the same field values, order and encoding as the Go
--     marshaller, so the bytes equal what the old relayer would have published;
--   * the old seq order is kept: the SDK relayer claims by (available_at,
--     created_at), so available_at is set to strictly increasing instants just
--     before now(), one microsecond apart, in seq order. occurred_at alone is not
--     enough, because concurrent writes can commit out of occurred_at order and
--     facts of one transaction share it. New writes get available_at = now() at
--     enqueue time, so they come after every copied row.
-- Re-running the copy is a no-op (ON CONFLICT DO NOTHING).

CREATE TABLE IF NOT EXISTS engagement_outbox_events (
    event_id       TEXT PRIMARY KEY,
    aggregate_type TEXT        NOT NULL,
    aggregate_id   TEXT        NOT NULL,
    event_type     TEXT        NOT NULL,
    payload        BYTEA       NOT NULL,
    principal      BYTEA,
    request_id     TEXT        NOT NULL DEFAULT '',
    status         TEXT        NOT NULL DEFAULT 'pending',
    attempts       INT         NOT NULL DEFAULT 0,
    available_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_until   TIMESTAMPTZ,
    published_at   TIMESTAMPTZ,
    error          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS engagement_outbox_events_claim_idx ON engagement_outbox_events (available_at, created_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS engagement_outbox_events_status_idx ON engagement_outbox_events (status);

-- Protobuf wire helpers (session-local). Field numbers here are all <= 15, so a tag
-- is one byte. Like the Go marshaller, a scalar that holds its zero value is left
-- out; a sub-message that is set is always written.
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

-- platform.events.v1.EventEnvelope as the old relayer built it: event_id = 1,
-- type = 2, occurred_at = 3 (Timestamp), principal = 4 (Principal, only when the
-- row recorded one), request_id = 6, payload = 7. principal_type holds the enum
-- name (PRINCIPAL_TYPE_ANONYMOUS = 1, _USER = 2, _SERVICE = 3).
INSERT INTO engagement_outbox_events
    (event_id, aggregate_type, aggregate_id, event_type, payload, request_id,
     status, attempts, available_at, created_at)
SELECT o.event_id::text,
       CASE WHEN o.type IN ('platform.engagement.v1.SellerFollowed', 'platform.engagement.v1.SellerUnfollowed')
            THEN 'Seller' ELSE 'Listing' END,
       o.key,
       o.type,
       pg_temp.pb_bytes(1, convert_to(o.event_id::text, 'UTF8'))
       || pg_temp.pb_bytes(2, convert_to(o.type, 'UTF8'))
       || pg_temp.pb_msg(3,
              pg_temp.pb_int(1, floor(extract(epoch FROM o.occurred_at))::bigint)
              || pg_temp.pb_int(2, ((extract(microseconds FROM o.occurred_at)::bigint % 1000000) * 1000)))
       || CASE WHEN o.principal_id = '' AND o.principal_type = '' THEN ''::bytea
               ELSE pg_temp.pb_msg(4,
                        pg_temp.pb_bytes(1, convert_to(o.principal_id, 'UTF8'))
                        || pg_temp.pb_int(2, CASE o.principal_type
                                                 WHEN 'PRINCIPAL_TYPE_ANONYMOUS' THEN 1
                                                 WHEN 'PRINCIPAL_TYPE_USER' THEN 2
                                                 WHEN 'PRINCIPAL_TYPE_SERVICE' THEN 3
                                                 ELSE 0 END)) END
       || pg_temp.pb_bytes(6, convert_to(o.request_id, 'UTF8'))
       || pg_temp.pb_bytes(7, o.payload),
       o.request_id,
       'pending',
       0,
       now() - ((count(*) OVER () - row_number() OVER (ORDER BY o.seq)) * interval '1 microsecond'),
       o.occurred_at
FROM outbox o
WHERE o.published_at IS NULL
ORDER BY o.seq
ON CONFLICT (event_id) DO NOTHING;

DROP FUNCTION pg_temp.pb_int(int, bigint);
DROP FUNCTION pg_temp.pb_msg(int, bytea);
DROP FUNCTION pg_temp.pb_bytes(int, bytea);
DROP FUNCTION pg_temp.pb_varint(bigint);
