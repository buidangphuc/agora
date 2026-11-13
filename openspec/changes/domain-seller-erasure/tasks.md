## 1. Code track (team-domain)

Depends on slice trust-legal-contract (generated stubs for `UserErased`; copy `identity.proto` and `events.proto` already present in team-domain).

- [ ] 1.1 Tests first (red) for the store method with Docker (`GO_SDK_REQUIRE_DOCKER=1`): all statuses deleted, variants cascade, 6 outbox rows, other seller untouched, 501-listing batch boundary, rollback on outbox failure, idempotent redelivery, no-listings no-op (scenarios: Erased seller's listings in every status are deleted, Other sellers' listings are not touched, Batch boundary of 500 listings, Store failure rolls back and retries, Redelivered event is a no-op, Second event for the same user is a no-op, Erasure of a user with no listings)
- [ ] 1.2 Tests first (red) for storefront, bundle and `seller_status_applied` deletion (scenarios: Storefront and bundles are deleted, Failed shop-data delete rolls back the listings too, Another seller's storefront and bundles survive)
- [ ] 1.3 `internal/store`: `PgTxWriter.EraseSellerListings(ctx, userID, eventID, enqueue)` in `seller_hold.go` style: dedupe in `processed_events`, batch select `FOR UPDATE`, `loadVariants`, delete, enqueue per listing, delete storefront/bundles/`seller_status_applied`, one commit (scenarios: Erased seller's listings in every status are deleted, Erasure also removes the seller's shop data, Erasure is idempotent and atomic)
- [ ] 1.3a Event row builder: `enqueueDeleted()` using `BuildListingChangedEnvelope` with `CHANGE_TYPE_DELETED` and principal `service-team-domain` (scenario: Delete events match the existing delete path)
- [ ] 1.4 Consumer tests first (red) with a fake store: malformed envelope, empty `user_id`, missing `erased_at`, other types ignored, roles not used, store error not permanent, no raw ids in logs (scenarios: Malformed event goes to the DLQ, Other event types are ignored, Roles in the event do not widen the delete, Logs carry no raw identifiers)
- [ ] 1.5 `internal/events/erasure.go`: `ErasureConsumer` mirroring `identity.go` (type `platform.identity.v1.UserErased`), hashed key logging per `consumer_log_test.go` (scenarios: The erasure consumer is isolated and fails safe)
- [ ] 1.6 Config `Events.ErasureGroup` (`KAFKA_ERASURE_GROUP`, default `team-domain-erasure`); wire a second `consume.NewConsumer` in `internal/app/app.go` with DLQ `identity.events.team-domain.dlq`; lifecycle run and close like the identity consumer (scenario: A poison erasure event does not block seller hold)
- [ ] 1.7 Compose: `KAFKA_ERASURE_GROUP` in `team-domain/docker-compose.local.yaml` and root `docker-compose.services.yaml`; add `identity.events.team-domain.dlq` to the topic init line (shared file, serialised by initiative-build) (scenario: Malformed event goes to the DLQ)
- [ ] 1.8 Regression: existing `DeleteListing` owner check unchanged (scenario: Erasure has no RPC entry point and owner checks stay)
- [ ] 1.9 Gates through `scripts/asdlc/gate_digest.py` for team-domain

## 2. E2E track (platform-e2e; FEATURES.yaml by e2e-author)

- [ ] 2.1 Feature file `domain/seller_erasure.feature` covering every scenario in `specs/domain-seller-erasure/spec.md`; replace each `VERIFIED BY: TBD` with its path and scenario title. Produce `UserErased` through the real identity deletion path once `identity-account-deletion` has landed, otherwise a Kafka test producer on `identity.events` (scenarios: all)
- [ ] 2.2 Step "listing leaves search within 60 s and GetListing is NotFound" reusing the seller-hold steps (scenario: Deleted listings leave the store)
- [ ] 2.3 Step that reads `identity.events.team-domain.dlq` for the malformed-event case (scenario: Malformed event goes to the DLQ, A poison erasure event does not block seller hold)
- [ ] 2.4 `team-domain/FEATURES.yaml` entry `listing.seller_erasure` (e2e-author)
