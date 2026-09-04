# 00-spec — Close the real gaps (backend greenfield + thin UI)

## Goal
The marketplace backend is already built end-to-end for most features. This change closes the *actual* gaps found by verification: build the 2 genuinely-missing backend capabilities (chat message search, recently-viewed history) end-to-end, wire thin UI onto 3 features whose backend+gateway are already done (order tracking timeline, returns/refunds, listing Q&A), and fix a 1-line autocomplete regression. UI stays minimal (revamp later).

## Non-goals
- No rebuild of already-complete features (reviews, alerts, search facets — all 4 layers done).
- No per-message chat read-receipts (thread-level `MarkThreadRead` already works). No new proto beyond the 2 RPCs below.
- No real payment/refund money movement (mock only, per AGENTS.md §7).
- No UI polish/redesign; no live-stack e2e or archive (handoff).

## Architecture decisions
- Recently-viewed lives in **team-engagement** (has per-user data + DB + existing `RecordView`), not team-analytics (event/warehouse only, no query API). Why: reuse the favorites/ListFavorites pattern and existing view path.
- Chat message search = ILIKE over `content`, scoped to the requesting participant's threads. Why: matches existing repo query style; no new infra.
- Gateway forwarders for the 2 new RPCs land in the **integration wave** (team-gateway is shared). Pattern: client in `internal/upstream/clients.go`, `callRead`/`callWrite` forwarder in `internal/edge/<svc>.go`, register in `internal/edge/server.go`.
- Proto is defined once up front (Wave 0) in platform-core; services vendor + `make proto`; frontend stubs regenerated in Wave 0.

## Contracts (defined Wave 0)
```proto
// platform/chat/v1/chat.proto — add to ChatService
rpc SearchMessages(SearchMessagesRequest) returns (SearchMessagesResponse);
message SearchMessagesRequest  { string query = 1; platform.common.v1.PageRequest page = 2; }
message SearchMessagesResponse { repeated ChatMessage messages = 1; platform.common.v1.PageResponse page = 2; }

// platform/engagement/v1/engagement.proto — add to EngagementService
rpc GetRecentlyViewed(GetRecentlyViewedRequest) returns (GetRecentlyViewedResponse);
message GetRecentlyViewedRequest  { platform.common.v1.PageRequest page = 1; }
message GetRecentlyViewedResponse { repeated string listing_ids = 1; platform.common.v1.PageResponse page = 2; }
```
`ChatMessage` already carries `thread_id`; `RecordView` gains a side-effect: append to a per-user `view_history` (dedup most-recent-first).

Already-existing contracts the UI tasks consume (no proto change):
- order/v1: `GetShipmentTracking`, `CreateShipment`, `GetSagaState`, `CreateReturnRequest`/`GetReturnRequest`/`UpdateReturnStatus`; payment/v1: `RefundPayment`.
- engagement/v1: `AskQuestion`, `AnswerQuestion`, `ListQuestionsByListing`.

## Conventions
- Go services: mirror an existing RPC in the same repo (handler → service → repository), colocated `*_test.go`, `go test ./...` or `make test`.
- Frontend: mirror `features/review/ReviewSection.tsx` + `lib/gateway/reviews.ts`; Vietnamese copy; named exports; Vitest colocated. Minimal styling.
- Do not touch team-gateway or another task's write-set. New deps forbidden.

## Locked
2026-09-04 — Confirmed by user ("Cả hai"). Scope: Wave 0 proto (chat SearchMessages, engagement GetRecentlyViewed) + regen; Wave 1 = 5 parallel tasks (T1 chat-search backend, T2 recently-viewed backend, T3 order post-purchase UI, T4 listing Q&A UI, T5 openfeature gitops); Wave 2 integration (gateway forwarders + greenfield FE + autocomplete fix + compose env). No per-message receipts; no rebuild of complete features; refunds mock; live-stack/archive = handoff.
