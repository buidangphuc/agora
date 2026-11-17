## Decisions
- Second consumer, not an extension of `IdentityConsumer`: the initiative convention is one consumer group and one DLQ per erasure concern (`identity.events.<svc>.dlq`). A poison `UserErased` then cannot block seller hold/restore and the reverse. Both consumers read the same topic; each ignores types it does not own (the existing handler already returns nil for unknown types).
- Delete by `seller_id` for every status, not only for users whose `roles` contain `seller`. A user who lost the role can still own listings, and the indexed query (`listings_seller_idx`) is cheap. `UserErased.roles` is therefore not used to filter.
- Reuse the delete shape of `deleteListing` (`DELETE ... RETURNING listingColumns`, `scanListing`, `loadVariants` before cascade) in batches of 500, same constant as `sellerHoldBatch`, inside one transaction like `applySellerStatus`. The outbox row builder is `BuildListingChangedEnvelope` with `CHANGE_TYPE_DELETED`, principal `service-team-domain`, so downstream consumers see exactly what `DeleteListing` emits.
- Variants must be loaded before the listing row is deleted (cascade), so the batch selects ids `FOR UPDATE`, loads variants, then deletes.
- Dedupe: `processed_events` by `event_id` (as hold/restore). Delete-by-seller is idempotent on its own, so a different event id for the same user is a no-op as well.
- Storefront, bundles and `seller_status_applied` of the user are deleted in the same transaction; `bundles.listing_ids` of other sellers are not touched.
- Reservations hold no user id and are left; reserve of a deleted listing already fails `NotFound`.
- Ordering vs hold/restore: after erasure a late `UserStatusChanged` finds no listings and is a no-op.
- The compose DLQ topic line is the only shared-file edit; consumer env goes in team-domain's `docker-compose.local.yaml` and the root `docker-compose.services.yaml`.
