## Why

initiative: trust-legal

When an account is erased, team-identity publishes `identity.events/UserErased` (contract slice
`trust-legal-contract`). team-engagement, team-chat and team-notification hold personal content keyed by the user id
(reviews, Q&A, favourites, follows, reports, chat messages, notifications) and today have no way to erase it. Q2 left
`reviews.hidden_by` / `hide_reason` for this work (`team-engagement/migrations/0011_review_integrity.up.sql:5-6`).
Backlog: Q24 (trust#10), user story 3 of the initiative.

## What changes

- team-engagement, team-chat and team-notification each add a Kafka consumer of `identity.events` for the
  `platform.identity.v1.UserErased` envelope type: own consumer group, DLQ `identity.events.<svc>.dlq`, one
  transaction per event, idempotent by user id; other event types on the topic are skipped.
- team-engagement: reviews, questions and answers keep rating and text, the author becomes "Người dùng đã xóa"
  (`user_name` rewritten, `user_id` re-keyed to the fixed value `deleted`); `hide_reason` cleared on the user's reviews;
  `hidden_by` cleared where it equals the user; favourites, follows (as follower and as followed seller), reports filed
  by the user, helpful votes cast by the user, wishlist collections and items, view history, check-ins and the loyalty
  account of the user deleted.
- team-chat: the user's messages get a fixed placeholder text, sender name "Người dùng đã xóa" and an emptied rich
  payload; threads stay for the counterparty; `last_message_text` is replaced when the last message was the user's.
- team-notification: the user's notifications, alert subscriptions and notification preferences are deleted.
- Compose wiring: DLQ topics `identity.events.engagement.dlq`, `identity.events.chat.dlq`,
  `identity.events.notification.dlq` in the topic init line and the consumer env of the three services.
- No new RPC, no proto change, no new dependency (franz-go and go-sdk `consume` are already used).

## Capabilities

### New Capabilities
- `engagement-user-erasure`: erasure of a user's reviews, Q&A, favourites, follows, reports and activity in team-engagement.
- `chat-user-erasure`: anonymisation of a user's chat messages in team-chat.
- `notification-user-erasure`: deletion of a user's notifications, alerts and preferences in team-notification.

### Modified Capabilities
none

## Impact

- `team-engagement`: `internal/events` (new identity consumer), `internal/store` (erase transaction), `internal/config`, `internal/app`, a migration only if the store needs an index on `follows.seller_id`/`reports.reporter_id` (both already indexed or scanned by primary key; see design).
- `team-chat`: `internal/events`, `internal/store`, `internal/config`, `internal/app`.
- `team-notification`: `internal/events`, `internal/store`, `internal/config`, `internal/app`.
- `docker-compose.services.yaml` (hot file shared with the other erasure slices; edits are additive).
- `platform-e2e`: new feature files, FEATURES.yaml entries (by the e2e author).
- Depends on `trust-legal-contract` for `UserErased` in the generated Go bindings.

## Non-goals

- Erasing data in other services (order, payment, domain, analytics, verification, audit): other slices.
- Disputes (`disputes.claimant_id` / `defendant_id` / `reason` / `evidence_urls`): not in the locked per-service rule; left
  untouched, see open issues in the run notes.
- Any new RPC, admin tool or UI; the author label is rendered by the existing `user_name` field.
- Restoring erased content: erasure is irreversible.

## Non-functional requirements

- Erasure of one user completes in each of the three services within 1 h of `UserErased` (local, DLQ empty); with a
  user holding 1000 rows across the tables the store transaction completes in <= 5 s.
- A transient store failure is retried up to 5 times, then the record goes to the DLQ; a malformed event goes to the DLQ at once.
- At-least-once delivery: replaying the same event any number of times leaves the same end state and no error.
- Each service boots only with Kafka and Postgres reachable (no `*_ENABLED` flag, no in-memory fallback).

## Data classification

| field | where | purpose | consent basis | after erasure |
|---|---|---|---|---|
| review `user_id`, `user_name` | team-engagement `reviews` | author of public review | contract with the buyer, then legitimate interest in keeping rating integrity | rewritten to `deleted` / "Người dùng đã xóa" |
| review/Q&A free text, `media_urls` | `reviews`, `product_questions`, `product_answers` | public product feedback | as above | kept (locked decision: text kept, author anonymised) |
| `hide_reason`, `hidden_by` | `reviews` | moderation record | legitimate interest; DPO sign-off pending (Q2 item) | `hide_reason` cleared on the erased user's reviews; `hidden_by` cleared where it is the erased user |
| favourites, follows, collections, view history, check-ins, loyalty, helpful votes, reports filed | team-engagement | personalisation, loyalty, moderation intake | consent / contract | deleted |
| chat message text, sender name, rich payload | team-chat `chat_messages` | delivery of conversation | contract | text replaced by fixed placeholder, name "Người dùng đã xóa", payload emptied |
| notification title/body, alert subscriptions, preferences | team-notification | delivery of notices | contract / consent | deleted |

Retention: personal data above is erased within 1 h of the event (and within the 72 h end-to-end window of the
initiative); no copy is kept by these services. The event log retention of Kafka and the immutable team-audit copies are
outside this slice. Neither raw user ids nor message text are logged: counts and hashed ids only.

## Threat model

- Assets: the user's authored content and activity history; the counterparty's thread history; other users' rows.
- Abuse cases: (1) a forged `UserErased` for a live user erases their data (mitigated: only the identity topic is
  consumed, producers are the identity outbox; consumers trust the topic ACL of the local stack, no RPC accepts the event);
  (2) over-broad update erases other users' rows or the counterparty's messages; (3) replay or reordering re-applies
  or half-applies erasure; (4) an erased id leaking through logs or DLQ payloads; (5) erasing a moderator's id breaks
  other users' hidden reviews.
- Authz rule: no principal-facing surface is added. The only input is the Kafka record on `identity.events`; every
  statement is scoped by the event's `user_id` (and, for chat, by `sender_id`), so no row of another user changes.
