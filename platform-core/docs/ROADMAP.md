# Roadmap — core ecommerce

Feature plan for a **standard ecommerce marketplace**. Scope here is the *core*
commerce surface (catalog → cart → order → pay → trust → seller ops). Heavy AI
(recommendation, semantic search, shopping assistant) is **out of scope for these
phases** — `team-ai` integrates in parallel on its own track and plugs into the
same gateway/events, so it never blocks a core phase.

**Ground rules for every feature below** (don't restate per row):
- Contract change is a **non-breaking** proto edit in `packages/proto` (add-only;
  `buf lint` + `buf breaking` pass), re-vendored + regenerated in the owning repo.
- Owning service **owns its tables** (Rule 3) + a DB migration; cross-service data
  is a gRPC call, never a DB join.
- State-change → emit a Kafka `<domain>.events` event (ADR-0002). Background job →
  RabbitMQ.
- Gateway gets a forwarder; RPCs gated by scope; `gofmt/vet/test` + `next build`
  clean; UI surfaced in `team-frontend`.

Status: ✅ done · 🔨 next · ⬜ planned

---

## Foundation — ✅ done
Auth + roles (team-identity) · Listing CRUD + owner (team-domain) · Lexical search +
suggest (team-search) · Favorites + views (team-engagement) · Edge gateway
(team-gateway) · Web SSR home + seller area + auth (team-frontend).

---

## Phase 3 — Catalog complete (real products)
| Feature | Requirement (acceptance) | Owner | New repo? | Status |
|---|---|---|---|---|
| Product images | Upload N images via presigned URL to object store (MinIO local / S3 prod); DB stores key only, no blob; card + detail show real images | team-domain + infra(MinIO) | No | 🔨 |
| Categories | Category tree; each product tagged ≥1 category; filter by category on home/search | team-domain | No | ⬜ |
| Variants + stock | Variant (color/size) with own price + stock; out-of-stock blocks purchase | team-domain | No | ⬜ |

## Phase 4 — Transactions (cart → order)
| Feature | Requirement (acceptance) | Owner | New repo? | Status |
|---|---|---|---|---|
| Shipping addresses | CRUD buyer addresses; pick a default | team-identity | No | ⬜ |
| Cart | Add/update/remove items; cart per user; live subtotal | **team-order** | **Yes** | ⬜ |
| Checkout → order | Create order from cart; **reserve stock** via gRPC to team-domain; state machine pending→paid→shipped→completed/cancelled | team-order | (same) | ⬜ |
| Order tracking | Buyer sees own orders; seller sees shop orders; seller advances status | team-order | (same) | ⬜ |

## Phase 5 — Payment & shipping (financially sensitive → mock)
| Feature | Requirement (acceptance) | Owner | New repo? | Status |
|---|---|---|---|---|
| Payment | ⚠️ **Mock only**: choose method, simulate "paid" callback → advance order state; NO real money movement | **team-payment** (mock) | **Yes** | ⬜ |
| Shipping fee | Fee by address (mock rate table); attach to order total | team-order | No | ⬜ |

## Phase 6 — Trust & interaction
| Feature | Requirement (acceptance) | Owner | New repo? | Status |
|---|---|---|---|---|
| Reviews & ratings | Only a buyer who purchased may review; average score shown on product + shop | team-engagement | No | ⬜ |
| Buyer↔seller chat | Realtime via SSE/WebSocket through gateway; message history persisted | gateway + **team-chat** | small: gateway · large: **Yes** | ⬜ |
| Notifications | Order/message/status events → in-app bell; sourced from Kafka events | **team-notification** | **Yes** (later) | ⬜ |

## Phase 7 — Seller ops, promotions, admin
| Feature | Requirement (acceptance) | Owner | New repo? | Status |
|---|---|---|---|---|
| Promotions / vouchers | Owns `vouchers/promo_rules/campaigns/redemptions`; seller creates shop-voucher, admin creates platform-campaign; order applies at checkout via `ValidateAndReserve → Commit/Release`; frontend shows badges | **team-promotion** (start as module in team-order, split when it grows) | Yes (eventually) | ⬜ |
| Seller dashboard + metrics | Revenue, orders, views/favorites over time (read from engagement/order via gRPC) | team-frontend | No | ⬜ |
| Admin | Approve/lock listings, manage users, handle reports | team-frontend (admin area) + identity/domain | No | ⬜ |

---

## Promotion — ownership note
`team-promotion` owns *the codes and discount rules*; `team-order` owns *the order
and final amount*. Order asks promotion "how much does this code cut, is it still
usable?" — it never holds promo rules or reads promo tables. Redemption counting is
kept consistent with reserve→commit/release, mirroring stock reservation:
```
order.Checkout → promotion.ValidateAndReserve(code, cart, buyer) → discount + reservationId
order PAID     → promotion.Commit(reservationId)      (idempotent)
order CANCEL   → promotion.Release(reservationId)
```

## New repos this roadmap introduces
`team-order` (Phase 4) · `team-payment`-mock (5) · maybe `team-chat` (6) ·
`team-notification` (6) · `team-promotion` (7, may start as a module in team-order).
Everything else extends an existing repo. Scaling *load* = more instances behind
the gateway, never a new repo.

## team-ai — parallel track (not a core phase)
Recommendation, hybrid/semantic search, and a RAG shopping assistant live in
`team-ai`. It consumes the same Kafka behavior events and plugs into the gateway,
so it can be built alongside any phase without blocking it. Wire it in once core
data (orders, views, catalog) is flowing.
