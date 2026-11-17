## Why
Account deletion (initiative trust-legal, Q24) publishes `identity.events/UserErased` when an account is erased. The analytics and ML stores hold rows keyed by the user id: `tracking_events.principal_id`, `order_facts.buyer_id`/`seller_id`, `engagement_facts.user_id`, `listing_sellers.seller_id`, the Parquet exports built from them, the featurestore online rows `fs:user_activity:*` and `fs:user_preferences:*`, recsys user vectors and recommendation caches, nearline recents, and team-ai chat sessions, quota and rate-limit counters. Without this slice these survive a deletion. This slice consumes `UserErased` in `team-analytics` (Go, DuckDB warehouse) and removes or re-keys every such row, and gives `platform-recsys`, `platform-featurestore` and `team-ai` the matching erasure.

initiative: trust-legal

## What changes
- team-analytics: a new `UserErased` consumer (own group, own DLQ) that, in one DuckDB transaction, re-keys the user's rows in `tracking_events`, `order_facts`, `engagement_facts` and `listing_sellers` to a random non-reversible pseudonym (deleting the user's favourite and follow facts), and records a tombstone so late events of that user are re-keyed on ingest. The next Parquet export contains no trace of the user id.
- platform-recsys: a new long-running `python -m recsys.erasure` consumer (own group, own DLQ) that deletes the user's Redis recommendation and nearline keys and the user's points in every `user_als_vectors__*` Qdrant collection.
- platform-featurestore: the job drops pseudonymised entities from its inputs, and `materialize` deletes online rows of user-keyed views whose entity is absent from the new snapshot. No Kafka client is added (see design).
- team-ai: an erasure addon on the existing aiokafka stack (own group, own DLQ) that deletes the user's chat session history, quota and rate-limit keys.
- Config keys, topic init lines for the DLQs and the consumer env in `docker-compose.services.yaml`.
- Consumes, unchanged: `identity.events/UserErased` (contract slice `trust-legal-contract`); no new RPC, no proto edit.

Out of scope: BigQuery adapter erasure (write-only, prod, deferred as local-first), rewriting historical offline featurestore snapshots and MLflow artifacts (see OPEN_ISSUES in the hand-off), GA4-side data, the `ga4_sessions` table (keyed by a GA client id, not a user id), team-audit.

## NFR
- Erasure of one user completes in this slice's consumers within 1 h of `UserErased` locally with every DLQ empty. team-analytics, recsys and team-ai consumers finish a user in under 60 s; Parquet files are clean within one `PARQUET_EXPORT_INTERVAL_SECONDS` (300 s in the root compose); featurestore online rows are clean after the first `materialize` run that starts after that export.
- At-least-once delivery, idempotent by user id; a failed purge commits no offset and is retried. A poison record goes to the DLQ and the consumer commits past it, so one bad record never blocks the topic.
- Availability: ingestion of `analytics.events` and `order.events` is not interrupted by an erasure; DuckDB writes are serialised by its single writer, the purge is one transaction.
- Limits: one event purges one user id; a user with up to 1 000 000 tracking rows is purged in one transaction.

## Data classification
| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `UserErased.user_id`, `roles` (input) | pseudonymous identifier of an erased person | drive erasure | legal obligation (data-subject erasure, Decree 13/2023, stricter of it and the 2026 PDP law; instruments unverified, NEEDS_HUMAN) | consumed, never stored in clear; logs carry a keyed fingerprint only |
| `tracking_events` (principal id, anonymous id, session id, `ga_client_id`, `properties`) | personal (behavioural, device identifiers) | analytics, recommendations | consent (analytics/personalization) | re-keyed to a random pseudonym on erasure; no mapping to the user is kept |
| `order_facts.buyer_id`/`seller_id`, `engagement_facts.user_id`, `listing_sellers.seller_id` | pseudonymous ids of the user | accounting aggregates, funnels | contract, accounting duty | re-keyed on erasure; amounts and ids of orders kept per the accounting period (NEEDS_HUMAN) |
| `erased_users` tombstone (HMAC of user id, pseudonym, erased_at) | pseudonymous; not reversible without `ERASURE_TOMBSTONE_SECRET` | re-key late events, prove erasure ran | legal obligation | kept as long as late events can arrive; period set by legal (NEEDS_HUMAN), default no expiry |
| Redis/Qdrant user keys (recs, nearline, featurestore, chat session, quota, rate-limit) | personal (preferences, chat text) | serving | consent / contract | deleted on erasure |
| Parquet exports, featurestore offline snapshots | personal until rebuilt | feature inputs | as above | exports rewritten each cycle; older offline snapshots: see OPEN_ISSUES |

## Threat model
- Assets: behavioural history and preferences of an erased person; chat text held in team-ai sessions.
- Abuse cases: (1) a forged `UserErased` erases another user's analytics: the topic is internal (not routed, produced only by the identity outbox) and the consumer never trusts payload roles for anything but logging. (2) Replay of an old `UserErased` after the user id was reused: ids are never reused. (3) Late or buffered events re-creating the erased id: closed by the tombstone re-key inside the insert transaction. (4) Re-identification from the pseudonym or tombstone: pseudonym is random (not derived), tombstone is HMAC-keyed. (5) Raw ids leaking into logs or the DLQ: logs use `kafkax` key fingerprints, the DLQ copy carries the original record as the existing DLQs do, and the envelope holds only the id already on the topic.
- Authorization rule: no RPC is added or changed; the only trigger is a record on `identity.events` read through the service's own consumer group. Each service touches only its own store (DuckDB file, its own Redis keys, its own Qdrant collections); none calls another service's database.
