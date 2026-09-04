-- 0006_outbox_events — transactional outbox (ADR-0002, DATA_ARCHITECTURE §2).
--
-- Every listing write inserts one row here IN THE SAME TRANSACTION as the
-- business write, so the event and the row commit or roll back together (no
-- dual-write hazard). A background relayer (internal/events/relayer.go) claims
-- pending rows with FOR UPDATE SKIP LOCKED, produces the stored EventEnvelope
-- bytes to `listing.events` keyed by aggregate_id, and marks them published.
--
-- `payload` holds the FULLY-marshalled platform.events.v1.EventEnvelope so the
-- wire bytes are byte-identical to the pre-outbox inline produce, and
-- `event_id` (the PK) is that envelope's event_id — a stable dedupe key that
-- survives re-delivery under at-least-once.

CREATE TABLE IF NOT EXISTS outbox_events (
    event_id       TEXT PRIMARY KEY,                      -- == EventEnvelope.event_id (consumer dedupe key)
    aggregate_type TEXT        NOT NULL,                  -- 'Listing'
    aggregate_id   TEXT        NOT NULL,                  -- listing_id; the Kafka partition key (ordering)
    event_type     TEXT        NOT NULL,                  -- e.g. 'platform.listing.v1.ListingChanged'
    payload        BYTEA       NOT NULL,                  -- marshalled EventEnvelope (produced verbatim)
    principal      BYTEA,                                 -- optional; the acting Principal is already carried inside payload
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
CREATE INDEX IF NOT EXISTS outbox_events_claim_idx
    ON outbox_events (available_at, created_at)
    WHERE status = 'pending';

-- Operational lookups by status (e.g. inspecting parked `failed` rows).
CREATE INDEX IF NOT EXISTS outbox_events_status_idx ON outbox_events (status);
