# [W1-T4] team-engagement — wishlist collections + richer reviews

## Role
SE   # one agent owns the whole engagement repo this wave (F3 collections + F4 reviews)

## Objective
EngagementService gains named wishlist Collections (CRUD + items) and enriched Reviews (media_urls, helpful_count, verified_purchase, MarkReviewHelpful, GetShopRatingSummary).

## Write-set (EXCLUSIVE)
- team-engagement/internal/handler/wishlist.go      (create + _test.go)
- team-engagement/internal/service/wishlist.go      (create + _test.go)
- team-engagement/internal/repository/wishlist.go   (create — Postgres + in-memory + _test.go)
- team-engagement/internal/handler/reviews.go       (edit/create — MarkHelpful, GetShopRatingSummary, media)
- team-engagement/internal/service/reviews.go       (edit/create)
- team-engagement/internal/repository/reviews.go    (edit/create)
- team-engagement/internal/upstream/order.go        (create — verified-purchase check via team-order gRPC)
- team-engagement/migrations/0004_wishlist_collections.up.sql / .down.sql (create)
- team-engagement/migrations/0005_review_enrichment.up.sql / .down.sql    (create)

## Read-only dependencies
- team-engagement generated stubs (Wave 0), existing favorites/reviews handler for pattern
- 00-spec.md §Contracts F3 (collections) + F4 (reviews)

## Contracts
Collections: per-user named lists; AddToCollection/RemoveFromCollection idempotent; ListCollections returns item_count. Reviews: `media_urls` stored on create; `MarkReviewHelpful` increments a per-user-unique helpful count (a user can't double-count); `verified_purchase` set by calling team-order (buyer has a delivered order for listing) — gRPC, NO DB join; `GetShopRatingSummary` aggregates across a seller's listings.

## Acceptance criteria
- [ ] Create collection, add/remove items, list items — happy path + idempotent add.
- [ ] Review created with media_urls; MarkReviewHelpful idempotent per user; helpful_count correct.
- [ ] verified_purchase true only when team-order confirms a delivered order (mock client in tests).
- [ ] GetShopRatingSummary returns correct average/count/breakdown across ≥2 listings.
- [ ] gofmt/go vet/go test clean; repo tests cover Postgres + in-memory.

## Review (gate — different agent)
Peer agent against SE rubric; **contract-boundary-reviewer** for the team-order upstream call.

## Verify
```bash
# in Docker: (cd team-engagement && gofmt -l . && go vet ./... && go test ./...)
```

## Out of scope
- Do NOT edit proto/generated/main.go/grpcserver (Wave 0). No alert subscriptions (that's team-notification, W1-T5). No gateway/frontend.
