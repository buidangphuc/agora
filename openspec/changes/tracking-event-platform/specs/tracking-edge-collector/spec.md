## Purpose

Defines what the gateway's `POST /api/track` collector guarantees as pure edge telemetry: per-event validation
against the taxonomy, size, batch and rate caps, consent handling, never forwarding network identity, server-side
time with clock-skew correction, and sampling knobs, without holding analytics logic or storage.

## ADDED Requirements

### Requirement: Each event is validated against the taxonomy and dropped individually

The collector SHALL validate every event of a batch against the taxonomy (known type, required fields present,
field formats and maximum lengths, registered placement) and SHALL produce the valid events and drop the invalid
ones, counting each drop by reason (`unknown_type`, `missing_required`, `invalid_field`, `unknown_placement`,
`too_large`, `too_old`, `opted_out`, `sampled_out`, `rate_limited`). A body that is not JSON, or in which every event
is invalid, SHALL get a 400 response; otherwise the response SHALL be 204.

#### Scenario: One bad event does not drop its neighbours

- **WHEN** a batch carries 3 valid events and 1 impression without `request_id`
- **THEN** the 3 valid events are produced, the invalid one is not, the response is 204, and the drop counter for
  `missing_required` increases by 1

#### Scenario: Oversized strings are rejected

- **WHEN** a `SEARCH` event carries a query longer than the taxonomy maximum
- **THEN** the event is dropped with reason `invalid_field`

### Requirement: Size, batch and rate caps protect the pipeline

The collector SHALL reject a body above 64 KiB (413), accept at most 50 events per request (extra events dropped
with reason `too_large`), at most 16 `properties` entries per event, and SHALL rate-limit per `anonymous_id`
(falling back to the in-memory client address when absent) with a configurable events-per-minute budget, answering
429 and producing nothing for a request over budget.

#### Scenario: A flood from one visitor is limited

- **WHEN** one `anonymous_id` sends more events per minute than the configured budget
- **THEN** requests over budget get 429, nothing from them is produced, and other visitors are unaffected

#### Scenario: A too-large body is refused

- **WHEN** a beacon body of 100 KiB arrives
- **THEN** the collector answers 413 and produces nothing

### Requirement: Network identity is never forwarded

The collector SHALL NOT put the client IP address, the full user agent, or any request header value other than the
session-derived principal into a produced event or envelope. The client address MAY be used only in memory for rate
limiting.

#### Scenario: No IP reaches Kafka

- **WHEN** a beacon arrives from a client with a forwarded IP header
- **THEN** no field of the produced envelope or payload, including `properties`, contains that IP address or the
  user-agent string

### Requirement: Event time is server-stamped with client clock-skew correction

The collector SHALL set `EventEnvelope.occurred_at` from the server clock, corrected by the client's own offset:
`occurred_at = received_at - (client_sent_at - client_occurred_at)` when the batch carries `sent_at` and the event
carries `client_occurred_at`, clamped to not be later than `received_at`; the raw client time SHALL be kept in
`client_occurred_at`. An event whose corrected time is older than the late-arrival bound (default 72 hours) SHALL be
dropped with reason `too_old`. A client clock far off (more than 24 hours) SHALL not move the event time.

#### Scenario: A client clock one hour fast is corrected

- **WHEN** a browser whose clock is one hour ahead sends an event it created 2 seconds before sending
- **THEN** the stored `occurred_at` is about 2 seconds before the gateway received it, and `client_occurred_at`
  holds the browser's raw time

#### Scenario: A queued event older than the bound is dropped

- **WHEN** a batch carries an event whose corrected time is 4 days ago
- **THEN** that event is dropped with reason `too_old`

### Requirement: Sampling is configurable, deterministic per session, and recorded

The collector SHALL support a sample rate per event type (default 1.0 for every type), decided deterministically from
`session_id` so a session is wholly in or out per type, and SHALL record the applied rate in `sample_rate` on every
produced event. `IDENTIFY`, `CONSENT_UPDATE`, `CHECKOUT_START` and `ADD_TO_CART` SHALL never be sampled.

#### Scenario: A sampled-out session produces no impressions

- **WHEN** the impression sample rate is 0.5 and a session hashes outside the sample
- **THEN** none of that session's impressions are produced and each is counted as `sampled_out`, while its clicks
  are produced with `sample_rate = 1`

#### Scenario: Identify is never sampled

- **WHEN** every event type's sample rate is set to 0.01
- **THEN** an `IDENTIFY` event is still produced
