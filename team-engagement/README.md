# team-engagement

Community, trust and post-order engagement service (Go, gRPC). It owns `engagement_db` (Postgres, Rule 3: nobody else connects to it) and serves `platform.engagement.v1.EngagementService` on `:50054`.

Bounded context: favorites, listing view/favorite stats and recently-viewed history, reviews and ratings (listing and shop level), wishlist collections, product Q&A, order disputes, seller follows, and loyalty daily check-in. Deployed: yes, in the root compose (`team-engagement` plus the one-shot `team-engagement-migrate`). Reached only through `team-gateway`.

---

## 1. Contract

Defined in `proto/platform/engagement/v1/engagement.proto` (vendored, see Gotchas). Authorization is `interceptor.RequireScopes` on the principal that the gateway forwards in `x-principal-id` / `x-principal-type` / `x-principal-scopes` metadata (ADR-0003; no token or secret in this service). A call with no principal is `Unauthenticated`; a missing scope is `PermissionDenied`. "Self" means the handler reads the user id from the principal, never from the request.

| RPC | Authorization | Notes |
|---|---|---|
| `AddFavorite`, `RemoveFavorite` | `engagement:write` | Self. Maintains `listing_stats.favorite_count`. |
| `IsFavorite`, `ListFavorites` | `engagement:read` | Self. Cursor paging. |
| `RecordView` | public (anonymous allowed) | Increments `view_count`; writes `view_history` only when a user id is present. |
| `GetListingStats` | public | |
| `GetRecentlyViewed` | none (self-scoped) | Anonymous callers get an empty list, no error. |
| `CreateReview` | `engagement:write` | Rating 1-5. Purchase verification: see below. |
| `ListReviews`, `GetListingRatingSummary`, `GetShopRatingSummary` | public | Shop summary aggregates by the denormalised `reviews.seller_id`. |
| `MarkReviewHelpful` | `engagement:write` | Idempotent per `(review_id, user_id)`. |
| `CreateCollection`, `AddToCollection`, `RemoveFromCollection` | `engagement:write` | |
| `ListCollections`, `ListCollectionItems` | `engagement:read` | |
| `AskQuestion` | `engagement:write` | |
| `AnswerQuestion` | `engagement:write` | `is_shop_reply` is honoured only when the caller also holds `listing.write`; otherwise it is stored as `false`. |
| `ListQuestionsByListing` | public | |
| `CreateDispute` | `engagement:write` | Claimant is the principal; `claimant != defendant`; status starts `OPEN`. |
| `GetDispute` | `engagement:read` | No party check: any reader with a dispute id can read it. |
| `ResolveDispute` | `engagement:write` **and** `admin` | Sets status to `INVESTIGATING`, `RESOLVED` or `REJECTED` plus a resolution text. |
| `FollowSeller`, `UnfollowSeller` | `engagement:write` | Cannot follow yourself. |
| `ListFollowedSellers`, `IsFollowing`, `ListFollowedListings` | `engagement:read` | The feed is newest first (`created_at` then listing id, descending) with an opaque keyset cursor; a malformed cursor is `InvalidArgument`. Needs the listing consumer (see Events). |
| `CheckIn` | `engagement:write` | Idempotent per calendar day; advances streak, awards coins. |
| `GetLoyalty` | `engagement:read` | |

Verified purchase (`CreateReview`): only when `UPSTREAM_ORDER_ADDR` is set **and** the request has an `order_id`, the service calls `team-order` `GetOrder` (`internal/upstream/order.go`). `verified_purchase = true` requires the order's `buyer_id` to equal the caller, status `ORDER_STATUS_COMPLETED`, and the order to contain the listing. The order's `seller_id` is stored on the review. If the address is empty, no `order_id` is given, or the upstream call fails, the review is saved unverified (upstream errors are logged as a warning, never returned).

Consumes: `team-order` `GetOrder` only (gRPC), called as the service principal `service-team-engagement` (type `service`, scope `order.read`; team-order requires a principal). No other upstream.

## 2. Events

Produces none. Consumes `listing.events` (`platform.events.v1.EventEnvelope`) when `KAFKA_ENABLED=true`; it is the only writer of `seller_listings` (the follow-feed source).

- `ListingChanged` with a `PUBLISHED` listing (created or updated): upsert `(seller_id, listing_id)`. `created_at` is the envelope's `occurred_at` and is kept on redelivery and later updates, so editing a listing does not bump it in the feed. A listing has one owning seller.
- `ListingChanged` `DELETED`, or any non-published status; and `ListingStatusChanged` to a non-published status: remove the row. `ListingStatusChanged` to `PUBLISHED` is ignored (it carries no seller id; the matching `ListingChanged` upserts).
- Delivery is at-least-once with idempotent writes. A failing record is retried with backoff (5 attempts), then parked on `<topic>.dlq`; if it cannot be parked the service exits without committing. A malformed record (bad envelope, no listing id, published without seller id) takes the same path.
- A new consumer group starts from the earliest offset, which backfills the feed. The consumer runs inside the server process (`cmd/server`), not as a separate binary.

## 3. Data

Own Postgres `engagement_db`, role `engagement_svc`. Tables by migration (`migrations/NNNN_*.up.sql` / `.down.sql`):

| Migration | Tables |
|---|---|
| 0001 | `favorites`, `listing_stats` |
| 0002 | `reviews` |
| 0003 | `product_questions`, `product_answers`, `disputes` |
| 0004 | `collections`, `collection_items` |
| 0005 | review enrichment (`media_urls`, `seller_id`, helpful count) and `review_helpful_votes` |
| 0006 | `view_history` |
| 0007 | `follows`, `seller_listings` |
| 0008 | `loyalty_accounts`, `checkins` |
| 0009 | indexes for the newest-first follow feed on `seller_listings` |

Migrations are applied by golang-migrate, never by the server. Root compose: the `team-engagement-migrate` one-shot runs before `team-engagement`. Standalone: `make migrate` (runs `migrate/migrate` in docker against `DATABASE_URL`, rewriting `localhost` to `host.docker.internal`).

## 4. Configuration

Read by `internal/config/config.go`; `.env.example` is the template.

| Variable | Default | Meaning |
|---|---|---|
| `ENV` | `local` | Environment name |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`/`warning`, `error` |
| `LOG_JSON` | `true` | JSON logs (else text) |
| `GRPC_HOST` | `0.0.0.0` | Bind host |
| `GRPC_PORT` | `50054` | Bind port (1-65535) |
| `GRPC_REFLECTION_ENABLED` | `true` | gRPC reflection |
| `SHUTDOWN_GRACE_SECONDS` | `10` | Graceful stop timeout |
| `DATABASE_ENABLED` | `true` | Must stay `true`: startup fails otherwise (the server has no in-memory mode) |
| `DATABASE_URL` | empty | Required when enabled. Standalone `postgresql://engagement_svc:engagement_pass@localhost:5436/engagement_db`; in root compose the host is `postgres:5432` |
| `DB_MAX_CONNS` | `10` | pgx pool size |
| `UPSTREAM_ORDER_ADDR` | empty | `team-order` gRPC address. Empty disables verified-purchase checks. Root compose sets `team-order-svc:50055` |
| `KAFKA_ENABLED` | `false` | Run the `listing.events` consumer that fills the follow feed. Off means the feed stays empty |
| `KAFKA_BROKERS` | `localhost:9092` | Comma-separated seed brokers (root compose: `redpanda:9092`) |
| `KAFKA_CONSUMER_GROUP` | `team-engagement-feed` | Consumer group |
| `KAFKA_LISTING_TOPIC` | `listing.events` | Topic; the DLQ is `<topic>.dlq` |
| `OTEL_ENABLED` | `false` | Tracing |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty (`.env.example` sets `http://localhost:4317`) | OTLP endpoint |
| `OTEL_SERVICE_NAME` | `team-engagement` | Service name in traces |

Drift gate: `make check-env` (`TestEnvExampleInSync`) fails if `.env.example` and the config struct disagree. Add a variable to both.

## 5. Run locally

Whole stack (root `docker-compose.yaml` includes `platform-core/infra` and `docker-compose.services.yaml`; one shared Postgres with a logical DB per service):

```bash
docker compose up -d --build        # from the repo root; migrate job runs first, then team-engagement
```

This service has no `--profile jobs` component.

Standalone (own Postgres on `:5436`, from `team-engagement/`):

```bash
make proto                                              # step 0: generated/ is gitignored
docker compose -f docker-compose.local.yaml up -d       # postgres-engagement
cp .env.example .env
make migrate
make run                                                # go run ./cmd/server
```

Optionally run `team-order` and set `UPSTREAM_ORDER_ADDR` to exercise verified purchases.

## 6. Build, test, lint

| Command | What |
|---|---|
| `make proto` | `buf generate` from `proto/` into `generated/` (needs `buf`; `buf.gen.yaml` uses remote plugins, so network access) |
| `make test` | `go test ./...` (plain `testing`, no testify) |
| `make check` | Merge gate: `check-env`, `gofmt -l`, `go vet ./...`, `go test ./...` |
| `make check-env` | `.env.example` drift test only |
| `make migrate`, `make run` | See above |

This repo has no CI workflow of its own; `make check` is the gate. Go 1.22 (`go.mod`; the Dockerfile builds on `golang:1.22` and runs on distroless nonroot).

## 7. Spec and verification

- `FEATURES.yaml` at the repo root lists 10 features (favorites, reviews, qa, disputes, collections, reviews-media, reviews-helpful, shop-rating-summary, reviews-verified-purchase, reviews-breakdown), each `automated` with a `covered_by` pointer into `platform-e2e/tests/e2e/features/engagement/`.
- Verify coverage from the repo root: `make -C platform-e2e features-check`, and for a change `make -C platform-e2e spec-check CHANGE=<id>`.
- Changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC.
- Follows, loyalty and recently-viewed have e2e feature files (`follow_seller.feature`, `loyalty_checkin.feature`) but no `FEATURES.yaml` entry.

## 8. Gotchas

- `generated/` is gitignored. `go build`, `go test` and `go run` fail on a fresh clone until `make proto` has run.
- `proto/` is a pinned vendored copy of platform-core's contract (ADR-0001). Never edit it here; change it in `platform-core/packages/proto` and re-vendor. Never hand-edit `generated/`.
- No in-memory fallback in the server binary. In-memory repositories exist only for tests.
- Public RPCs (`RecordView`, `GetListingStats`, the list/summary reads) accept anonymous callers; `GetRecentlyViewed` returns empty for them.
- An empty `UPSTREAM_ORDER_ADDR` silently disables purchase verification (`.env.example` ships it empty).
- The `DATABASE_ENABLED=false` startup error text in `internal/bootstrap/lifecycle.go` says "team-identity" (copy-paste); the behaviour is correct.

## 9. Known gaps

- **Follow feed depends on the consumer.** It is empty unless `KAFKA_ENABLED=true` and the group has read `listing.events`. Root compose does not set the Kafka variables for this service yet. A stale redelivery of an old `PUBLISHED` event after a `DELETED` can re-add a listing (events for one listing are keyed together, so this needs a replay or rebalance race).
- **Shop replies depend on a scope, not ownership.** The service does not know which seller owns a listing, so `AnswerQuestion` honours `is_shop_reply` for any caller with `listing.write`, even on another seller's listing.
- **`GetDispute` has no party check.** Any caller with `engagement:read` and a dispute id can read it.
- **Dispute state machine is loose.** `ResolveDispute` accepts `INVESTIGATING`, `RESOLVED` or `REJECTED` from any non-closed state, so `OPEN` can go straight to `RESOLVED` and `INVESTIGATING` can be set repeatedly. Closed disputes return `FailedPrecondition`. `CreateDispute` does not verify the order or the parties against `team-order`.
- **Review display name is synthetic.** `CreateReview` sets `user_name` to `"User " + first 6 chars of id` (or a Vietnamese placeholder for short ids).
- Root `AGENTS.md` describes this service as "Favorites, stats, reviews, Q&A, disputes"; it omits wishlist, follows and loyalty.

## 10. Links

- Root rules: [`../AGENTS.md`](../AGENTS.md) (Rule 3 DB-per-service, Rule 4 contract is source of truth).
- ADRs (`../platform-core/docs/ADR/`): `0001-proto-distribution.md`, `0003-auth-model.md`, `0004-observability-otel.md`.
