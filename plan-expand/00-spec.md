# 00-spec — Expansion batch (6 features from the 4-PM backlog)

## Goal
Add 6 more net-new features, each extending one existing service (one per distinct repo → structurally disjoint), then bring the full stack up locally for browser testing. Backend + thin UI per feature. Mock money.

## Non-goals
No new service repos this batch. No UI polish. No rebuild of existing features. Live-stack e2e/archive = handoff; but a local `docker compose up --build` IS in scope (post-scrum) so the user can test.

## Architecture decisions
- Each feature = new RPC(s) in its domain proto + implement in that service (handler→service→repository + migration), mirroring an existing RPC in the repo. Gateway forwarders + thin FE = integration wave.
- Toolchain: Go 1.27 + buf at /opt/homebrew/bin. Regen via make proto / buf generate. buf.build BSR may rate-limit — stubs pre-vendored in Wave 0.

## Features (owner | RPCs | reference)
- **F1 Account Safety Center** — team-identity | `ListSessions`, `RevokeSession`, `ListLoginHistory` | mirror an existing identity RPC. Tables: sessions, login_history.
- **F2 Saved Searches** — team-search | `SaveSearch`, `ListSavedSearches`, `DeleteSavedSearch`, `RunSavedSearch` | mirror SearchListings. Table: saved_searches(user_id, query, filters jsonb).
- **F3 Reorder ("Mua lại")** — team-order | `Reorder(order_id)` re-adds a past order's items to the cart (returns Cart) | mirror CartService.AddToCart + GetOrder.
- **F4 Loyalty check-in** — team-engagement | `CheckIn` -> {streak, coins_earned}, `GetLoyalty` -> {streak, coin_balance, last_checkin} | mirror favorites. Tables: loyalty_accounts, checkins.
- **F5 Bundle deals** — team-domain(listing) | `CreateBundle`, `GetBundle`, `ListBundlesBySeller` | mirror listing CRUD. Table: bundles(id, seller_id, title, listing_ids[], bundle_price int64).
- **F6 Sponsored listings** — team-promotion | `CreateAdCampaign(listing_id, budget, bid)`, `ListSponsoredSlots(context)` -> sponsored listing_ids | mirror voucher. Table: ad_campaigns. Mock credits (no real money).

## Contracts
Lists use platform.common.v1.PageRequest/PageResponse; times google.protobuf.Timestamp; money int64 minor-units. Request/response-message-per-RPC, matching existing protos.

## Conventions
Go: mirror named-RPC + handler→service→repository + colocated *_test.go; make test / go test. No new deps. Thin UI mirrors features/review/ReviewSection.tsx. Vietnamese copy. Don't touch team-gateway/team-frontend from Wave 1 (integration only).

## Locked
2026-09-04 — user chose "batch from backlog (~6)" + "docker compose up --build". Wave 1 = F1-F6 (extend, 6 parallel, disjoint repos), Wave 2 = integration (gateway forwarders + thin UI), then local stack up for testing. Mock money; no new service repos.
