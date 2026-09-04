# 00-spec — 12 features from the 4-PM roundtable

## Goal
Ship 12 net-new marketplace features in parallel: 8 that extend an existing service (one per distinct repo) + 4 brand-new service repos. Each feature = new proto RPC(s) + backend(+DB) + thin UI. Maximize feature count while keeping write-sets structurally disjoint (one repo per task).

## Non-goals
- No rebuild of already-built features. No UI polish (thin only; revamp later). No real money (wallet/payout/subscription are mock, AGENTS.md §7).
- No live-stack e2e / archive in-scrum (handoff). New-repo services get gitops+compose wiring in the integration wave only.

## Architecture decisions
- **Tier-A** = add RPC(s) to an existing domain proto + implement in that service, mirroring an existing RPC in the repo. **Tier-B** = new proto package + new Go service repo scaffolded by mirroring `team-notification` (simplest committed-stub Go service: cmd/server, internal/{handler,service,repository,config}, Dockerfile, Makefile, buf.gen.yaml, migrations).
- Gateway forwarders + thin FE + gitops/compose for new services = integration wave (shared seams).
- Financial features (wallet, payout, subscription) are mock ledgers, no money movement.
- Toolchain: Go 1.27 + buf at /opt/homebrew/bin (installed). Services regen via `make proto`/`buf generate`; frontend via `npm run proto`.

## Features (owner repo | indicative RPCs | reference)
### Wave 1 — Tier-A (extend existing repo)
- **F1 Follow Seller** — team-engagement | `FollowSeller,UnfollowSeller,ListFollowedSellers,IsFollowing,ListFollowedListings` | mirror ListFavorites. Migration `follows`.
- **F2 Rich chat messages** — team-chat | extend `ChatMessage` (MessageType enum + payload: LISTING_CARD, QUICK_REPLY); `SendMessage` gains `message_type`+`payload` | mirror SendMessage.
- **F3 Notification prefs + digest** — team-notification | `GetNotificationPrefs,UpdateNotificationPrefs` (+digest scheduler) | mirror alerts. Migration `notification_prefs`.
- **F4 Seller analytics** — team-analytics | ADD a query gRPC service: `GetSellerFunnel,GetRevenueBreakdown` (reads warehouse tables) | mirror another Go grpcserver; analytics currently health+consumer only.
- **F5 Seller storefront** — team-domain(listings) | `UpsertStorefront,GetStorefront` (by seller/slug) | mirror listing CRUD. Migration `storefronts`.
- **F6 Seller wallet (mock)** — team-payment | `GetWalletBalance,ListLedgerEntries,RequestPayout` | mirror payment handlers. Migration `wallet_ledger`.
- **F7 Subscription tiers** — team-promotion | `ListPlans,Subscribe,GetEntitlements` | mirror voucher service. Migration `subscriptions`.
- **F8 AI review summarization** — team-ai (Python) | `SummarizeReviews(reviews[])->{summary,pros,cons}` | mirror existing team-ai completion/magic-listing module (FastAPI + optional ai/v1 RPC).

### Wave 2 — Tier-B (new service repo; scaffold by mirroring team-notification)
- **F9 team-referral** | `CreateReferralCode,GetMyReferral,RedeemReferral,ListReferralRewards` | new proto `referral/v1`.
- **F10 team-sharing** | `CreateShareLink,ResolveShareLink` (short link + OG meta) | new proto `sharing/v1`.
- **F11 team-audit** | `WriteAuditEvent,QueryAuditLog` (append-only) | new proto `audit/v1`.
- **F12 team-verification** | `SubmitKyc,GetVerificationStatus,ReviewKyc` (mock KYC + badge) | new proto `verification/v1`.

## Contracts
All RPCs use `platform.common.v1.PageRequest/PageResponse` for lists, `google.protobuf.Timestamp` for times, int64 minor-units for money, and mirror the request/response-message-per-RPC style of existing protos. Exact messages authored in Wave 0 against existing conventions.

## Conventions
- Go: mirror the named-RPC + handler→service→repository + colocated `*_test.go` pattern; `go test ./...` or `make test`. Python (team-ai): ruff + pytest, `make test`.
- No new deps beyond a feature's inherent need. Thin UI mirrors `features/review/ReviewSection.tsx`. Vietnamese copy.
- Do not touch team-gateway or another task's repo. Generated code never hand-edited.

## Locked
2026-09-04 — user chose "12 (8 Tier-A + 4 new-repo)". Wave 1 = F1-F8 (extend, 8 parallel), Wave 2 = F9-F12 (new repos, 4 parallel), Wave 3 = integration (gateway forwarders, thin UI, gitops+compose for new services, verify). Mock money; no rebuilds; live-stack/archive = handoff.
