CREATE TABLE IF NOT EXISTS promotion_outbox (
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
ALTER TABLE promotion_outbox ADD COLUMN IF NOT EXISTS topic       TEXT;
ALTER TABLE promotion_outbox ADD COLUMN IF NOT EXISTS traceparent TEXT;
CREATE INDEX IF NOT EXISTS promotion_outbox_status_idx ON promotion_outbox (status);
CREATE INDEX IF NOT EXISTS promotion_outbox_agg_idx ON promotion_outbox (aggregate_type, aggregate_id, seq) WHERE status <> 'published';
CREATE INDEX IF NOT EXISTS promotion_outbox_pending_idx ON promotion_outbox (seq) WHERE status = 'pending';
