-- 0003_chat_outbox — transactional outbox for chat.events (ADR-0002, ADR-0013).
--
-- SaveMessage inserts the chat message, updates the thread AND inserts one row here
-- IN THE SAME TRANSACTION, so a stored message always yields its event (even if
-- Kafka is down when it is sent) and a message that fails to store yields none.
-- A background relayer claims pending rows with FOR UPDATE SKIP LOCKED, produces
-- the stored EventEnvelope bytes to `chat.events` keyed by aggregate_id (thread_id),
-- and marks them published. Same shape as team-order's order_outbox_events.

CREATE TABLE IF NOT EXISTS chat_outbox_events (
    event_id       TEXT PRIMARY KEY,                      -- == EventEnvelope.event_id (consumer dedupe key)
    aggregate_type TEXT        NOT NULL,                  -- 'ChatThread'
    aggregate_id   TEXT        NOT NULL,                  -- thread_id; the Kafka partition key (ordering)
    event_type     TEXT        NOT NULL,                  -- 'platform.chat.v1.ChatMessage'
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

CREATE INDEX IF NOT EXISTS idx_chat_outbox_claim
    ON chat_outbox_events (available_at, created_at)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_chat_outbox_aggregate
    ON chat_outbox_events (aggregate_id, created_at);
