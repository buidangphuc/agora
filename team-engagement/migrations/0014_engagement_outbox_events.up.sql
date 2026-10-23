CREATE TABLE IF NOT EXISTS engagement_outbox_events (
    event_id       TEXT PRIMARY KEY,
    aggregate_type TEXT        NOT NULL,
    aggregate_id   TEXT        NOT NULL,
    event_type     TEXT        NOT NULL,
    payload        BYTEA       NOT NULL,
    topic          TEXT,
    request_id     TEXT        NOT NULL DEFAULT '',
    traceparent    TEXT,
    seq            BIGSERIAL,
    status         TEXT        NOT NULL DEFAULT 'pending',
    attempts       INT         NOT NULL DEFAULT 0,
    available_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_until   TIMESTAMPTZ,
    published_at   TIMESTAMPTZ,
    error          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE engagement_outbox_events ADD COLUMN IF NOT EXISTS topic       TEXT;
ALTER TABLE engagement_outbox_events ADD COLUMN IF NOT EXISTS traceparent TEXT;
CREATE INDEX IF NOT EXISTS engagement_outbox_events_status_idx ON engagement_outbox_events (status);
CREATE INDEX IF NOT EXISTS engagement_outbox_events_agg_idx ON engagement_outbox_events (aggregate_type, aggregate_id, seq) WHERE status <> 'published';
CREATE INDEX IF NOT EXISTS engagement_outbox_events_pending_idx ON engagement_outbox_events (seq) WHERE status = 'pending';

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

INSERT INTO engagement_outbox_events
    (event_id, aggregate_type, aggregate_id, event_type, payload, request_id,
     status, attempts, available_at, created_at)
SELECT o.event_id::text,
       CASE WHEN o.type IN ('platform.engagement.v1.SellerFollowed', 'platform.engagement.v1.SellerUnfollowed')
            THEN 'Seller'
            WHEN o.type = 'platform.engagement.v1.DisputeResolved' THEN 'Dispute'
            ELSE 'Listing' END,
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

UPDATE engagement_outbox_events
SET status = 'pending', attempts = 0, available_at = now(), locked_until = NULL, error = NULL
WHERE status = 'failed';
