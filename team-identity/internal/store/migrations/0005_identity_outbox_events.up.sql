-- 0005_identity_outbox_events — transactional outbox (ADR-0002, ADR-0003 addendum).
--
-- RevokeSession flips sessions.revoked and inserts one row here IN THE SAME
-- TRANSACTION, so the revocation and its SessionRevoked event commit or roll back
-- together (no dual-write hazard). A background relayer (internal/events/relayer.go)
-- claims pending rows with FOR UPDATE SKIP LOCKED, produces the stored
-- EventEnvelope bytes to `identity.events` keyed by aggregate_id (the user id), and
-- marks them published. Same shape as team-domain's outbox_events.
--
-- `payload` holds the FULLY-marshalled platform.events.v1.EventEnvelope and
-- `event_id` (the PK) is that envelope's event_id — a stable dedupe key under
-- at-least-once delivery.

CREATE TABLE IF NOT EXISTS identity_outbox_events (
    event_id       TEXT PRIMARY KEY,                      -- == EventEnvelope.event_id
    aggregate_type TEXT        NOT NULL,                  -- 'Session'
    aggregate_id   TEXT        NOT NULL,                  -- user_id; the Kafka partition key
    event_type     TEXT        NOT NULL,                  -- 'platform.identity.v1.SessionRevoked'
    payload        BYTEA       NOT NULL,                  -- marshalled EventEnvelope (produced verbatim)
    request_id     TEXT        NOT NULL DEFAULT '',       -- trace continuity
    status         TEXT        NOT NULL DEFAULT 'pending',-- pending | published | failed
    attempts       INT         NOT NULL DEFAULT 0,        -- incremented on each relay failure
    available_at   TIMESTAMPTZ NOT NULL DEFAULT now(),    -- backoff gate
    locked_until   TIMESTAMPTZ,                           -- lease held by a claiming relayer
    published_at   TIMESTAMPTZ,                           -- set on successful produce
    error          TEXT,                                  -- last failure reason
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Claim query support: only pending rows are scanned, ordered (available_at, created_at).
CREATE INDEX IF NOT EXISTS identity_outbox_claim_idx
    ON identity_outbox_events (available_at, created_at)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS identity_outbox_status_idx ON identity_outbox_events (status);
