## Purpose

Bounds heavy first-party tracking: random first-party identifiers only, no fingerprinting, free-text scrubbing, PII
classes with retention windows, opt-out honoured at the edge, per-user erasure across the warehouse, and a published
statement of what is collected.

## ADDED Requirements

### Requirement: Identifiers are first-party, random and never derived from the device

The platform SHALL identify visitors only with the random `anonymous_id` and `session_id` it sets itself, SHALL NOT
load third-party tracking scripts or pixels, and SHALL NOT compute or collect any fingerprint (canvas, fonts, audio,
hardware, IP-derived id or combination of browser attributes).

#### Scenario: The tracking payload carries no device characteristics

- **WHEN** any client event is produced
- **THEN** its payload contains no screen size, user agent, language list, timezone fingerprint, IP address or
  other device attribute, and its `properties` keys are all registered in the taxonomy

### Requirement: Free text is scrubbed before it is stored

The system SHALL scrub `search_query` (and any free-text property) of email addresses, Vietnamese phone numbers and
keyword-anchored national id numbers (CMND/CCCD), using the same rules as the AI service's redaction, replacing them
with `[email]`, `[phone]` and `[id]`; SHALL strip query strings and fragments from `page_path` and reduce an external
`referrer` to its host. Scrubbing SHALL happen at the gateway before produce, so unscrubbed text never reaches Kafka.

#### Scenario: A query containing a phone number is masked

- **WHEN** a buyer searches "iphone 13 liên hệ 0912 345 678"
- **THEN** the stored `search_query` is "iphone 13 liên hệ [phone]"

#### Scenario: A price is not mistaken for an id

- **WHEN** a buyer searches "tủ lạnh 850000000"
- **THEN** the stored `search_query` is unchanged

#### Scenario: An external referrer keeps only its host

- **WHEN** a visitor arrives from `https://www.google.com/search?q=my+email@x.com`
- **THEN** the stored `referrer` is `www.google.com`

### Requirement: Opt-out is honoured at the edge

When a visitor has opted out (cookie `bds_consent=analytics:0`, or the browser sends `Sec-GPC: 1`) the SDK SHALL stop
sending behavioural events and the gateway SHALL drop any behavioural event it still receives (reason `opted_out`).
The only event accepted from an opted-out visitor SHALL be `CONSENT_UPDATE`, which records the choice and carries no
behavioural data. In `opt_in` consent mode, behavioural events SHALL be dropped until consent is granted. The gateway
SHALL forward the visitor's computed consent state (`granted` or `denied`) to upstream services on every call as
gateway-computed metadata, ignoring any client-supplied value, so serving can stop personalising for an opted-out
visitor.

#### Scenario: An opted-out visitor produces no behavioural events

- **WHEN** a visitor opts out and then searches, views and adds to cart
- **THEN** none of those events reach `analytics.events` or the warehouse, and the actions themselves succeed

#### Scenario: Global Privacy Control is respected

- **WHEN** a browser sends `Sec-GPC: 1` with a beacon of impressions
- **THEN** the gateway produces none of them

#### Scenario: The consent state reaches upstream services and cannot be spoofed

- **WHEN** a visitor with `Sec-GPC: 1` loads a page whose server render calls `Recommend`, and the request also
  carries a client header claiming consent is granted
- **THEN** the upstream call carries the consent state `denied`

#### Scenario: Opting back in resumes tracking

- **WHEN** an opted-out visitor turns tracking back on and views a listing
- **THEN** a `CONSENT_UPDATE` and the subsequent `VIEW` land in the warehouse

### Requirement: Each PII class has a retention window enforced by the warehouse

Every stored column SHALL belong to one PII class (`none`, `pseudonymous`, `free_text`, `fact`), and a scheduled
retention job, the only component that deletes or nulls raw event rows, SHALL enforce the configured windows (defaults: `free_text` columns nulled after 90 days; behavioural
rows deleted after 13 months; identity links deleted 13 months after last seen; quarantine rows after 14 days; fact
rows after 25 months). Retention runs SHALL be recorded with the number of rows affected.

#### Scenario: Old search text is nulled but the event remains

- **WHEN** the retention job runs over a `search` event that occurred 91 days ago
- **THEN** its `search_query` is null and its other columns are unchanged

#### Scenario: Rows past the window are deleted

- **WHEN** the retention job runs over behavioural rows older than 13 months
- **THEN** those rows no longer exist and the run is recorded with the deleted count

### Requirement: A user's data can be erased on request

An admin-only erasure RPC SHALL delete, for a given user id, every warehouse row whose principal is that user, every
row carrying an `anonymous_id` linked to that user, the user's identity links and the user's fact rows (or, for
facts needed for seller revenue reporting, replace the user id with a non-reversible tombstone), SHALL record the
erasure (hashed subject, time, rows affected) and SHALL add the subject to a tombstone list that later ingestion and
downstream exports honour, so re-delivered events for the subject are not stored again.

#### Scenario: Erasure removes a user's rows

- **WHEN** buyer B browsed anonymously, logged in, browsed more, placed an order, and an admin erases B
- **THEN** inspecting by B's user id or B's linked `anonymous_id` returns no rows, the order fact no longer carries
  B's id, and the erasure is recorded

#### Scenario: Replayed events for an erased user are not restored

- **WHEN** the analytics consumer replays `analytics.events` after B's erasure
- **THEN** no row for B or B's linked `anonymous_id` reappears

### Requirement: What is collected is documented

The platform SHALL publish a collection statement in `platform-core/docs` generated from the taxonomy (each event, each
field, its purpose, PII class and retention) and SHALL fail the taxonomy check when the statement is stale.

#### Scenario: A new field without documentation fails the check

- **WHEN** a field is added to the taxonomy and the collection statement is not regenerated
- **THEN** the taxonomy consistency check fails
