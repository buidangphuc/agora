-- 0006_order_outbox — transactional outbox for order events (ADR-0002, ADR-0013).
--
-- On PAID transition, team-order updates orders.status AND inserts one row here
-- IN THE SAME TRANSACTION, so the order state and the emitted event commit or
-- roll back together (no dual-write hazard). A background relayer claims pending
-- rows with FOR UPDATE SKIP LOCKED, produces the stored EventEnvelope bytes to
-- `order.events` keyed by aggregate_id (order_id), and marks them published.

CREATE TABLE IF NOT EXISTS order_outbox_events (
    event_id       TEXT PRIMARY KEY,                      -- == EventEnvelope.event_id (consumer dedupe key)
    aggregate_type TEXT        NOT NULL,                  -- 'Order'
    aggregate_id   TEXT        NOT NULL,                  -- order_id; the Kafka partition key (ordering)
    event_type     TEXT        NOT NULL,                  -- e.g. 'platform.order.v1.OrderPaid'
    payload        BYTEA       NOT NULL,                  -- marshalled EventEnvelope (produced verbatim)
    request_id     TEXT        NOT NULL DEFAULT '',       -- trace continuity
    status         TEXT        NOT NULL DEFAULT 'pending',-- pending | published | failed
    attempts       INT         NOT NULL DEFAULT 0,        -- incremented on each relay failure
    available_at   TIMESTAMPTZ NOT NULL DEFAULT now(),    -- backoff gate: claim only rows available_at <= now()
    locked_until   TIMESTAMPTZ,                           -- lease held by a claiming relayer
    published_at   TIMESTAMPTZ,                           -- set on successful produce
    error          TEXT,                                  -- last failure reason
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()     -- insert order / observability
);

CREATE INDEX IF NOT EXISTS idx_order_outbox_claim
    ON order_outbox_events (available_at, created_at)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_order_outbox_aggregate
    ON order_outbox_events (aggregate_id, created_at);
