# Replay: `plans/backend` through initiative-plan steps 1-2 (W2-8 evidence)

Date: 2026-10-11. (`plans/backend/` was removed in the 2026-10-14 workspace cleanup; read it at commit `6e17300b`.) Dry run, read-only on `plans/backend/`: no OpenSpec change was written, no
subagent was called (step 3 team spec and step 4 review were not executed). Only
`plans/_template/00-spec.md` was added; the slices below exist only in this note.

## Input
`plans/backend/` (parallel-scrum, Locked 2026-09-04): Wave 0 proto (chat `SearchMessages`,
engagement `GetRecentlyViewed`), Wave 1 = 5 tasks T1-T5 split by repo, Wave 2 integration
(gateway forwarders + FE greenfield + autocomplete fix + compose env).

## Step 1 — 00-spec
Accepted as is: it has `## Locked` and no `[UNRESOLVED` tag. Gaps against the new template
(`plans/_template/00-spec.md`): no `Capabilities → slices` table, no NFR / personal-data section
(recently-viewed history is per-user behavioural data → `pii` routing), no hot-file list
(team-gateway `internal/edge/server.go`, `internal/upstream/clients.go` were known shared).

## Step 2 — slices.yaml the skill would write
Rule applied: one slice = one user-observable capability = one OpenSpec change, vertical
(service + gateway + FE tasks inside the slice so its e2e can go green); proto in a contract slice.

| slice | owner | depends_on | uses / provides | v1 tasks it absorbs |
|---|---|---|---|---|
| backend-gaps-contract | platform-core | – | provides ChatService.SearchMessages, EngagementService.GetRecentlyViewed | W0 |
| chat-message-search | team-chat | backend-gaps-contract | uses ChatService.SearchMessages | T1 + W2 gateway forwarder + W2 FE search UI |
| recently-viewed-history | team-engagement | backend-gaps-contract | uses EngagementService.GetRecentlyViewed, RecordView | T2 + W2 forwarder + W2 "Vừa xem" widget + `recordView` call |
| order-postpurchase-ui | team-order | – | uses OrderService.{GetShipmentTracking, GetSagaState, Create/Get/UpdateReturnRequest}, PaymentService.RefundPayment | T3 |
| listing-qa-ui | team-engagement | – | uses EngagementService.{AskQuestion, AnswerQuestion, ListQuestionsByListing} | T4 |
| *(out of initiative)* | – | – | – | T5 = task §3 of the existing change `wire-openfeature`; autocomplete 1-line fix; compose env no-op |

`cross_change_check.py --slices <scratch copy>`: `dag` and `contract` clean (every `uses` exists in
the proto tree today); 5 `schema` violations "no change directory" — expected, step 3 not run.
Build order printed: backend-gaps-contract → chat-message-search → listing-qa-ui →
order-postpurchase-ui → recently-viewed-history.

Waves initiative-build would derive (validate_plan disjointness): W0 contract alone; W1
chat-message-search ∥ order-postpurchase-ui ∥ listing-qa-ui; W2 recently-viewed-history (shares
gateway `server.go`/`clients.go` with chat-message-search and FE `lib/gateway/engagement.ts` with
listing-qa-ui). v1 had 5 parallel tasks because it pushed every gateway/FE edit into one
integration wave; v2 trades one extra wave for slices that are each e2e-complete.

## Versus what was actually done
- **No OpenSpec change was ever created or archived for this work.** None of
  `SearchMessages`, `GetRecentlyViewed`, `AskQuestion`, `ListQuestionsByListing` appears in
  `openspec/changes/archive/**` or `openspec/specs/**`. It shipped straight from parallel-scrum
  packets; `99-release.md` left e2e, FEATURES flips and archive as a handoff.
- Coverage today: no FEATURES.yaml entry for chat message search; recently-viewed has no
  feature id of its own. Q&A (`engagement.qa`), shipment tracking and returns (`order.shipment-tracking`,
  `order.rma-return`) are covered, but through later changes, not this plan.
- Later archived changes that touched the same surfaces: `2026-09-04-wire-openfeature` (T5's
  parent), `2026-10-08-payment-refund-model` (returns/refunds model), `2026-10-09-ui-phase-orders`
  (rewrote `OrderTimeline` / `ReturnRequestSection` from T3), `2026-10-09-ui-phase-product-detail`
  (Q&A state, T4), `2026-10-09-ui-phase-discovery` (recently viewed widget + autocomplete),
  `2026-10-09-engagement-fact-events` (explicitly leaves Q&A and recently-viewed out of preference signals).
- So the replay maps 5 slices onto 0 archived changes 1:1: v1 split by repo/task (T1-T5 + integration),
  v2 splits by capability. The two backend slices would have produced the spec + e2e that the
  handoff never delivered; the two UI slices were later redone by the ui-phase-* changes.

## Stop point
initiative-plan would now run step 3 (5 team-spec-authors in one message), step 4 review, and stop
at `plans/backend-gaps/LOCK.md` (pending). Not executed in this dry run.

## Open points found by the replay
- No script builds `decide.py` units from a change (spec / feature gates): the skills ask the
  orchestrator to hand-build `<slice>.spec-units.json`; a `units.py` helper would remove that step.
- `owner` for UI-only slices (order-postpurchase-ui → team-order) relies on the FEATURES.yaml
  ownership convention; cross_change_check `ownership` will confirm it only once the deltas exist.
