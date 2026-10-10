## Why

This is the first change of the AI-first wave. The user approved the plan on 2026-10-08: seven changes in sequence,
re-specified on agora's design. The recommendation and feature work downstream trusts `tracking_events`, but the edge
collector and the warehouse sink let bad or duplicated data through:
- One unknown event type rejects the whole batch, so a single bad beacon drops up to 99 good events.
- The envelope `event_id` is minted by the gateway on every publish. A browser that re-sends a beacon (`keepalive`
  retry, a flush racing `beforeunload`) is therefore stored twice, and the warehouse anti-join on `event_id` cannot see
  the duplicate.
- Free text is stored verbatim: `search_query`, `page_path`, `referrer` and `properties` values. A buyer who types an
  email address or phone number into search puts personal data into the warehouse and into every training set.
- Nothing bounds the size of `properties` or of the string fields beyond the 64 KiB body cap.
- The warehouse has no record of when it wrote a row. Late or replayed events are indistinguishable from fresh ones,
  and freshness cannot be measured.
- Visitors browse anonymously and then log in. No resolved view links their anonymous history to the account, so
  features and training see two unrelated visitors.

## What Changes

- **team-gateway (collector):**
  - Validate each event on its own: invalid ones are dropped, the rest are produced. The response reports accepted and
    dropped counts; a body with no valid event is still a 400.
  - Bound the field sizes.
  - Derive the envelope `event_id` deterministically from the visitor and a client-supplied `eventId`, so a re-sent
    beacon dedupes in the warehouse.
  - Scrub emails, phone numbers and long digit runs from the free-text fields.
  - Reduce the referrer to its origin and path.
- **team-frontend (tracker):**
  - Give every beacon a `eventId` (UUID).
  - Re-send a failed `fetch` flush once with the same ids.
  - Anonymous id storage (localStorage) is unchanged.
- **team-analytics (warehouse):**
  - Add an `ingested_at` column.
  - Add two views:
    - `tracking_identity` maps each anonymous id to the principal it last logged in as.
    - `tracking_events_resolved` adds a `user_key`: the principal when known, else the stitched principal, else the
      anonymous id.
- **platform-e2e:** scenarios through the edge, checking `analytics.events` and the warehouse file.

Repos touched: team-gateway, team-frontend, team-analytics, platform-e2e. **No proto change**: the client event id
travels in the beacon only and becomes the envelope `event_id`.

## Capabilities

### New Capabilities
- `tracking-ingest-integrity`: per-event validation and bounds, idempotent re-sends, free-text scrubbing, ingest time,
  and identity stitching for tracking events.

### Modified Capabilities
- `tracking`: the collector requirement now drops invalid events one by one instead of rejecting the whole batch.

## Non-goals

- **Consent, retention and erasure** (legal is deferred by the user): no consent header, no consent gating, no
  retention job, no erase RPC. Anonymous id storage is unchanged, and no new cookie is set.
- **New event types** (`page_view`, `identify`) and any `analytics.proto` change.
- **Data-quality metrics and alerts.** That is the next change, `analytics-data-quality`.
- **Engagement facts** (`engagement-fact-events`) and anything feature-store or recsys related.
- **Client clock and skew correction.** `occurred_at` stays the edge receive time, which is already authoritative.

## Impact

- **Collector response:** `POST /api/track` now answers 202 with `{"accepted": n, "dropped": m}` instead of 204.
  `sendBeacon` ignores the body, and the frontend `fetch` fallback treats any 2xx as success.
- **Warehouse:**
  - New nullable column `ingested_at`, added by the existing idempotent `ALTER ... ADD COLUMN IF NOT EXISTS`.
  - Two views, created idempotently.
  - Existing rows have a null `ingested_at`.
- **Downstream:** consumers that read `tracking_events` can move to `tracking_events_resolved` for the user key. The
  recsys trainer moves in `featurestore-datasets`.
