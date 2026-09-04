# 99-release — deploy delta for the 4 ecommerce features

Only the DELTA vs the repo's existing process. CI/CD unchanged: `deploy/act` builds images + bumps tags → Gitea → ArgoCD auto-sync. No new pipeline.

## New service: team-promotion (:50061)
A brand-new bounded context (AGENTS.md §6c), registered wherever services are declared:
- **GitOps (DONE, not pushed):** `argocd/apps/team-promotion.yaml` (local app) + `envs/services/team-promotion.yaml` (staging/prod baseline, networkPolicy allowFrom [team-gateway, team-order]) + added to `appset-services.yaml` list + `team-promotion: local` in `envs/local/values.yaml` + appended to Vault `SERVICES`. Cross-service env also wired in both gitops layers: gateway→promotion+notification, order→promotion, engagement→order, notification→listing.events. Validated via `helm template` (renders clean). **Changes are staged in the platform-gitops working tree but NOT committed/pushed** — pushing to the gitea repo triggers ArgoCD auto-sync (a deploy), left for explicit authorization.
- **Compose (done):** service block + `team-promotion-migrate` job in `docker-compose.services.yaml`; `postgres-promotion` + `postgres-notification` in `platform-core/infra/docker-compose.yaml`.
- **AGENTS.md (done):** §4 port table row + ports line.

## New infra (done in compose; mirror in real env)
- `postgres-promotion` (host :5440, db `promotion_db`) — owns vouchers, voucher_reservations, flash_sale_campaigns.
- `postgres-notification` (host :5441, db `notification_db`) — **team-notification had NO DB before**; now owns notifications + alert_subscriptions. Prod must provision this DB.
- Kafka topics (added to `redpanda-init`): `promotion.events`, `listing.events`, `listing.events.dlq`.

## New env vars (set in the real deployment)
- **team-order:** `UPSTREAM_PROMOTION_ADDR=team-promotion-svc:50061` (voucher redemption in saga).
- **team-gateway:** `UPSTREAM_PROMOTION_ADDR`, `UPSTREAM_NOTIFICATION_ADDR` (NotificationService was newly edge-exposed).
- **team-engagement:** `UPSTREAM_ORDER_ADDR=team-order-svc:50055` (verified-purchase check) — **required**, else `verified_purchase` is always false.
- **team-notification:** `DATABASE_URL` (postgres-notification), `KAFKA_ENABLED=true`, `KAFKA_BROKERS`, `LISTING_EVENTS_TOPIC=listing.events`, `NOTIFICATION_LISTING_CONSUMER_GROUP`, `LISTING_EVENTS_DLQ_TOPIC`.
- **team-promotion:** `DATABASE_URL` (postgres-promotion), `FEATURE_FLAGS_ENABLED`, `FLIPT_ADDR`, `KAFKA_ENABLED`, `KAFKA_BROKERS`, `PROMOTION_EVENTS_TOPIC=promotion.events`.

## New migrations to run
- team-promotion `0001_promotion`
- team-order `0005_voucher_redemption`
- team-engagement `0004_wishlist_collections`, `0005_review_enrichment`
- team-notification `002_alert_subscriptions` (and `001` if the DB is fresh)

## Proto (additive, buf breaking-clean)
`platform/promotion/v1` (new) + additive fields/RPCs on `search`, `engagement`, `notification`, `order` in `platform-core`. Each consumer re-vendored + regenerated.

## Ops notes / known issues fixed this session
- **team-search EnsureIndex** made idempotent (was crash-looping the indexer on cold-boot index race). Real prod risk removed.
- **opensearch networking (local only):** the local infra opensearch was orphaned off `platform-core_default`; in a clean environment the compose brings it up correctly. Not a code change.
- **Feature flags:** promotion uses `flash-sale-enabled` (Flipt), fail-open. Register it in the flag store if you want a kill-switch.

## Verify live (per-feature)
- Voucher: apply at checkout → discounted total. Flash-sale: listing shows sale price + live meter.
- Search: facet sidebar with counts; facet click narrows results.
- Wishlist: create collection + add/remove; review photo + helpful vote + verified badge (needs UPSTREAM_ORDER_ADDR).
- Alerts: subscribe on a listing → seller lowers price → price-drop notification appears.
