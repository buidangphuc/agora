# 99-release — Handoff (needs live stack + deploy auth)

This scrum is **code-complete and host-verified**; the steps below need a running stack or a deploy and are intentionally NOT executed here.

## What shipped (host-verified)
- **Contracts** (platform-core): `chat/v1 SearchMessages`, `engagement/v1 GetRecentlyViewed` — `buf lint` clean; stubs regenerated in team-chat, team-engagement, team-gateway, team-frontend.
- **Backend**: chat message search (team-chat, `go test ./...` green), recently-viewed history (team-engagement, `make test` green + migration `0006_view_history`).
- **Frontend (thin)**: order tracking timeline + returns/refunds (mock), listing Q&A, chat message-search UI, recently-viewed "Vừa xem" widget, autocomplete bug fix. `tsc` clean; vitest green except the pre-existing `src/lib/track.test.ts` (unrelated localStorage-in-jsdom failures).
- **Gateway**: forwarders for the 2 new RPCs (`make check` green).
- **Infra**: `wire-openfeature` §3 gitops `values.env` (team-order flipt:9000 gRPC, team-frontend flipt:8080 REST). Compose env batch was already wired.

## Handoff steps (run when the stack is up + authorized)
1. **Bring up the stack**: `docker compose -f docker-compose.services.yaml up --build` (Go builds run their own `make proto`; DBs apply migrations incl. `view_history`).
2. **DB migrations**: confirm team-engagement `0006_view_history` applied (recently-viewed needs it; `GetRecentlyViewed` returns empty without data).
3. **E2E + coverage flips** (`platform-e2e`): add scenarios for chat search, recently-viewed, order tracking/returns, Q&A; `make -C platform-e2e features-check`; flip the relevant `team-*/FEATURES.yaml` entries to `automated` once green.
4. **OpenFeature deploy**: the gitops `values.env` change (T5) auto-syncs via ArgoCD — **this triggers a deploy**, so land it only with explicit authorization.
5. **Commit hygiene**: the working trees carried substantial pre-existing uncommitted changes from prior scrums *before* this work; separate this scrum's commits from that backlog when committing.

## Notes / follow-ups
- `payment/v1 RefundPayment` exists in platform-core + backend + gateway but the frontend's vendored payment proto is stale, so refunds are surfaced as a client-side mock routed through the real `order.UpdateReturnStatus → REFUNDED` (correct per AGENTS.md §7 mock rule). To wire the real RPC later: vendor payment.proto into team-frontend, `npm run proto`, add a `refundPayment` gateway wrapper.
- Per-message chat read-receipts (seen/delivered ticks) remain a future contract change (thread-level `MarkThreadRead` already works).
