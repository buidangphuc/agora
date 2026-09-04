-- 0003_payment_outbox — transactional outbox for payment events (ADR-0002, ADR-0009).
--
-- On settle, team-payment updates payment_transactions.status AND inserts one
-- row here IN THE SAME TRANSACTION, so the payment state and the emitted event
-- commit or roll back together (no dual-write hazard, replaces the old
-- fire-and-forget order.UpdateOrderStatus RPC). A background relayer
-- (internal/events/relayer.go) claims pending rows with FOR UPDATE SKIP LOCKED,
-- produces the stored EventEnvelope bytes to `payment.events` keyed by
-- aggregate_id (order_id), and marks them published.
--
-- `payload` holds the FULLY-marshalled platform.events.v1.EventEnvelope, and
-- `event_id` (the PK) is that envelope's event_id — a stable dedupe key that
-- survives re-delivery under at-least-once.

CREATE TABLE IF NOT EXISTS payment_outbox_events (
    event_id       TEXT PRIMARY KEY,                      -- == EventEnvelope.event_id (consumer dedupe key)
    aggregate_type TEXT        NOT NULL,                  -- 'Payment'
    aggregate_id   TEXT        NOT NULL,                  -- order_id; the Kafka partition key (ordering)
    event_type     TEXT        NOT NULL,                  -- e.g. 'platform.payment.v1.PaymentSettled'
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

-- Claim query support: only pending rows are ever scanned, ordered by
-- (available_at, created_at) — matches ClaimPending's ORDER BY.
CREATE INDEX IF NOT EXISTS payment_outbox_events_claim_idx
    ON payment_outbox_events (available_at, created_at)
    WHERE status = 'pending';

-- Operational lookups by status (e.g. inspecting parked `failed` rows).
CREATE INDEX IF NOT EXISTS payment_outbox_events_status_idx ON payment_outbox_events (status);
