## Why
When an account is erased (initiative trust-legal, Q24), identity publishes `UserErased` on `identity.events`. A seller's listings carry the seller id, title, description and images, so they are personal data of an erased person and must go. The initiative decision is final: team-domain deletes an erased seller's listings through the existing delete path (one `ListingChanged(DELETED)` per listing) and does not keep them as archived. Search, analytics and team-ai already react to `ListingChanged` DELETED, so no other service changes for listings.

initiative: trust-legal

## What changes
- New consumer in team-domain for `platform.identity.v1.UserErased` on `identity.events`, with its own consumer group `team-domain-erasure` and DLQ `identity.events.team-domain.dlq`, mirroring `internal/events/identity.go`. The existing `UserStatusChanged` consumer (group `team-domain-identity`, DLQ `identity.events.dlq`) is unchanged and keeps ignoring event types it does not know.
- New store method `EraseSellerListings` on `PgTxWriter`: in one transaction, dedupe by event id in `processed_events`, delete every listing whose `seller_id` is the erased user in any status (variants go by `ON DELETE CASCADE`), write one `ListingChanged(DELETED)` outbox row per deleted listing attributed to principal `service-team-domain`, delete the user's `storefronts` row, `bundles` rows and `seller_status_applied` row.
- Config `KAFKA_ERASURE_GROUP` (default `team-domain-erasure`) next to `KAFKA_IDENTITY_GROUP`; DLQ topic is derived like the existing one. No `*_ENABLED` flag.
- Compose: `docker-compose.services.yaml` topic init gains `identity.events.team-domain.dlq` (hot file shared with every erasure slice; this slice adds only its own line).
- No new RPC, no proto change (contract comes from `trust-legal-contract`), no new dependency, no migration beyond what is needed (none expected: all deleted tables exist).

Out of scope: deleting listing image objects in the `listing-images` bucket (the existing `DeleteListing` path does not delete objects either); rejecting `CreateListing` from a still-valid access token of an erased seller; any change in search, analytics or team-ai.

## NFR
- Erasure of one seller completes in team-domain within 1 h of `UserErased` locally (initiative NFR); target for a seller with 1000 listings is under 60 s in one transaction, batches of 500.
- Idempotent: redelivery of the same event, or a second event for the same user, deletes 0 rows and writes 0 outbox rows.
- Availability: consumer is at-least-once; store failure is retried, malformed events go to the DLQ and never block the partition.
- Search and listing reads stop returning the listings within 60 s of the event (outbox relay plus search consumer).

## Data classification
| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `listings` row of an erased seller (`seller_id`, title, description, `image_keys`, variants) | personal (seller-authored content linked to the user id) | none after erasure | erasure on data-subject request (Decree 13/2023, 2026 PDP law; instrument unverified, NEEDS_HUMAN) | deleted in the erasure transaction |
| `storefronts` (slug, tagline, banner) and `bundles` of the seller | personal (public shop identity) | none after erasure | same | deleted in the erasure transaction |
| `seller_status_applied.user_id` | pseudonymous id | enforcement ordering | legitimate interest | deleted in the erasure transaction |
| `processed_events.event_id` | non-personal | dedupe | n/a | existing purge window |
| `ListingChanged(DELETED)` payload | the deleted listing snapshot, includes `seller_id` | let search, analytics, team-ai drop the listing | same as erasure | Kafka topic retention (`listing.events`); their own slices delete derived rows |
| listing image objects in MinIO | personal if the image shows a person | none | same | NOT deleted by this slice (open issue) |

## Threat model
- Assets: the seller's published content and the integrity of other sellers' listings.
- Abuse cases: a forged or replayed `UserErased` deletes another seller's listings (only the identity service writes `identity.events`; the consumer deletes strictly by the event `user_id`, never by roles or by pattern); a malformed event stalls the partition (permanent error goes to DLQ); a redelivered event double-writes outbox rows (event-id dedupe plus delete-by-seller is naturally idempotent); a partial failure leaves half the listings (single transaction, rolled back, retried); an event for a user with no listings or a non-seller is a harmless no-op.
- Authz rule: no RPC is added; there is no caller. The only trigger is a Kafka message on `identity.events` of envelope type `platform.identity.v1.UserErased`. Listing owner checks on `DeleteListing` are untouched.
