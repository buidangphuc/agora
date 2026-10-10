# tracking-ingest-integrity Specification

## Purpose
Defines how tracking events are validated one by one, made idempotent across re-sends, scrubbed of personal data
at the edge, timestamped at ingest, and stitched from anonymous browsing to the account that logs in.

## Requirements

### Requirement: Each event in a batch is validated on its own

`POST /api/track` SHALL validate every event of a batch independently. An event is invalid when:
- its type is unknown; or
- any field breaks these bounds:
  - `listingId`, `sessionId`, `anonymousId`, `placementId`, `impressionId`, `modelVersion`, `eventGroupId`,
    `transactionId`: 128 characters each;
  - `path` and `referrer`: 512;
  - `query`: 256;
  - `properties`: at most 20 keys, each key at most 40 characters and each value at most 256.

Invalid events SHALL be dropped and the valid ones produced. The response SHALL be HTTP 202 with
`{"accepted": <n>, "dropped": <m>}`. A body that is not JSON, exceeds the body cap, holds more than 100 events or holds
no valid event SHALL be refused with HTTP 400, and nothing SHALL be produced for it.

#### Scenario: One bad event does not drop the batch

- **WHEN** a visitor posts a batch of three events, two valid views carrying unique markers and one of type
  "teleport"
- **THEN** the response is 202 with accepted 2 and dropped 1, and both valid markers reach `analytics.events`

#### Scenario: An oversized properties map drops only that event

- **WHEN** a visitor posts a batch with one valid view and one view whose `properties` has 21 keys
- **THEN** the response is 202 with accepted 1 and dropped 1, and only the valid view reaches `analytics.events`

### Requirement: A re-sent event is stored once

A beacon MAY carry `eventId`, a UUID chosen by the client. When it is a valid UUID, the gateway SHALL derive the
envelope `event_id` deterministically from the visitor key (the verified principal id, else the beacon's
`anonymousId`) and the `eventId`. The same event re-sent by the same visitor therefore carries the same `event_id`.
Without a valid `eventId`, the gateway SHALL mint a random `event_id` as before. The warehouse sink SHALL keep at most
one row per `event_id`. The storefront SHALL send a fresh `eventId` with every event, and SHALL re-send a flush whose
`fetch` failed at most once, with the same ids.

#### Scenario: Posting the same event twice stores one row

- **WHEN** a visitor posts the same view (same `eventId` and `anonymousId`, unique listing id) twice
- **THEN** both envelopes on `analytics.events` carry the same `event_id`, and the warehouse holds exactly one row for
  that listing id

#### Scenario: Two visitors cannot collide on an event id

- **WHEN** two different anonymous visitors post views with the same `eventId`
- **THEN** the warehouse holds one row for each visitor

#### Scenario: The storefront sends an event id on every beacon

- **WHEN** a buyer opens a product page in the storefront
- **THEN** every beacon the page sends to `/api/track` carries a distinct UUID `eventId`

### Requirement: Free text is scrubbed of personal data at the edge

Before producing, the gateway SHALL replace the following in `search_query`, `page_path` and every `properties`
value: email addresses with `[email]`, Vietnamese phone numbers (`0` or `+84` followed by 9 digits, with spaces or
dots allowed) with `[phone]`, and any run of 13 or more digits with `[number]`. It SHALL reduce `referrer` to its
scheme, host and path, dropping the query string and fragment. Prices, listing ids and other short numbers SHALL be
kept.

#### Scenario: An email and a phone in a search query are masked

- **WHEN** a visitor posts a view whose `query` is "a.b@example.com 0912 345 678 tủ lạnh 850000"
- **THEN** the event on `analytics.events` has `search_query` "[email] [phone] tủ lạnh 850000"

#### Scenario: A referrer loses its query string

- **WHEN** a visitor posts a view whose `referrer` is "https://news.example.com/a/b?utm_source=x&email=a@b.co#top"
- **THEN** the event on `analytics.events` has `referrer` "https://news.example.com/a/b"

### Requirement: The warehouse records when it ingested each row

The warehouse sink SHALL set `ingested_at` on every tracking row it writes, using its own clock at write time.
`occurred_at` SHALL remain the time the edge received the event.

#### Scenario: A stored event has an ingest time after its occurrence

- **WHEN** a visitor posts one view with a unique listing id and the sink writes it
- **THEN** the warehouse row has a non-null `ingested_at` that is not earlier than its `occurred_at`

### Requirement: Anonymous history is stitched to the account that logs in

The warehouse SHALL provide two views:
- `tracking_identity`, mapping each `anonymous_id` that appeared on events with exactly one USER principal to that
  principal. The anonymous id comes from the beacon body, so an id seen with more than one account is ambiguous and SHALL
  NOT be mapped; a logged-in user cannot claim another visitor's anonymous history by replaying its id;
- `tracking_events_resolved`, with every `tracking_events` column plus `user_key`, which is the event's USER principal
  when present, else the principal `tracking_identity` maps its `anonymous_id` to, else `anon:<anonymous_id>`.

#### Scenario: Pre-login browsing resolves to the user after login

- **WHEN** a visitor posts a view anonymously with `anonymousId` X, then logs in as a new buyer and posts a view with
  the same `anonymousId` X
- **THEN** in `tracking_events_resolved` both views have `user_key` equal to the buyer's id

#### Scenario: An anonymous id seen with two accounts is not stitched

- **WHEN** a visitor posts a view anonymously with `anonymousId` Z, then two different new buyers each post a view with
  the same `anonymousId` Z
- **THEN** in `tracking_events_resolved` the anonymous view has `user_key` "anon:Z" and each buyer's view has that buyer's
  id

#### Scenario: A never-logged-in visitor keeps an anonymous key

- **WHEN** a visitor posts a view anonymously with a fresh `anonymousId` Y and never logs in
- **THEN** in `tracking_events_resolved` that view has `user_key` "anon:Y"
