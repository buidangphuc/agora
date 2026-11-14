# LOCK — monetization
status: approved
approved-by: Bùi Đăng Phúc
date: 2026-11-13
spec: plans/monetization/00-spec.md (Locked 2026-11-13)

## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| monetization-contract | platform-core | [] | 1 | pass | pass (1 PERM warning, accepted) | pass | ENGINE_DOWN (LLM fallback) | f4e79137debd | [x] |
| order-paid-price-breakdown | team-order | [monetization-contract] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | d46c34ca75da | [x] |
| payment-settlement-commission | team-payment | [order-paid-price-breakdown] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | d5732d11bc16 | [x] |
| payment-seller-fee-charge | team-payment | [monetization-contract] | 1 | pass | pass (2 PERM warnings, false positive) | pass | ENGINE_DOWN (LLM fallback) | 6ca8f809d7d5 | [x] |
| seller-fee-breakdown-ui | team-frontend-seller | [payment-settlement-commission] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 874f4f22a608 | [x] |
| promotion-plan-billing | team-promotion | [payment-seller-fee-charge] | 1 | pass | pass (1 PERM warning on sweeper) | pass | ENGINE_DOWN (LLM fallback) | aa23a86f4f7c | [x] |
| listing-plan-cap | team-domain | [monetization-contract] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 7447b7996677 | [x] |
| seller-plan-ui | team-frontend-seller | [promotion-plan-billing, listing-plan-cap] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 0448636b6787 | [x] |
| promotion-ads-serving | team-promotion | [monetization-contract] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | ca7ecb76bc52 | [x] |
| promotion-ads-campaign-mgmt | team-promotion | [promotion-ads-serving] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 0f165738f7b7 | [x] |
| buyer-sponsored-slots | team-frontend | [promotion-ads-serving] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 97a32ec5baae | [x] |
| seller-ads-ui | team-frontend-seller | [promotion-ads-campaign-mgmt] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 5a7e6ce9b756 | [x] |

decide.py was not run for any slice: it needs a `--units` file and the kev engine, and the system python lacks `yaml`. Each spec-quality-reviewer's own checklist is the verdict.

## Build order (cross_change_check)
Topological order from cross_change_check.py:
monetization-contract -> listing-plan-cap -> order-paid-price-breakdown -> payment-seller-fee-charge -> payment-settlement-commission -> promotion-ads-serving -> buyer-sponsored-slots -> promotion-ads-campaign-mgmt -> promotion-plan-billing -> seller-ads-ui -> seller-fee-breakdown-ui -> seller-plan-ui

Proposed waves (deps + hot-file serialisation; initiative-build re-checks write-sets with validate_plan.py):
0. monetization-contract (contract, sequential, alone)
1. order-paid-price-breakdown | payment-seller-fee-charge | listing-plan-cap | promotion-ads-serving
2. payment-settlement-commission (after payment-seller-fee-charge: ledger.go + migration CHECK) | promotion-ads-campaign-mgmt | buyer-sponsored-slots
3. promotion-plan-billing (after promotion-ads-campaign-mgmt: promotion migrations, sponsored.go, edge/promotion.go) | seller-fee-breakdown-ui | seller-ads-ui
4. seller-plan-ui (after seller-ads-ui: lib/gateway/promotion.ts)

Hot-file pairs that must not run in parallel (from 00-spec):
- team-payment/internal/store/ledger.go, team-payment/migrations/ (type/sign CHECK): payment-settlement-commission x payment-seller-fee-charge. The second to land merges the other's CHECK arm additively (COMMISSION, COMMISSION_REVERSAL, SUBSCRIPTION_FEE).
- team-promotion/internal/store/migrations/ (numbering), internal/sponsored/sponsored.go, internal/store/sponsored.go: promotion-plan-billing x promotion-ads-serving x promotion-ads-campaign-mgmt. Migration numbers (plan-billing placeholder 0009) are renumbered at build time.
- team-gateway/internal/edge/promotion.go: promotion-plan-billing x promotion-ads-serving x promotion-ads-campaign-mgmt. edge/payment.go: payment-settlement-commission only.
- docker-compose.yaml (UPSTREAM_PAYMENT_ADDR for team-promotion, UPSTREAM_PROMOTION_ADDR for team-domain): promotion-plan-billing x listing-plan-cap.
- team-frontend-seller/src/lib/gateway/promotion.ts: seller-plan-ui x seller-ads-ui. lib/gateway/payment.ts: seller-fee-breakdown-ui only.

## Cross-change check
OK (exit 0), 0 violations (initial run had 2; fixed by renaming the duplicate scenario titles in buyer-sponsored-slots and seller-ads-ui).

## Open issues / NEEDS_HUMAN
Escalated slices: none (all passed in 1 round; promotion-ads-campaign-mgmt in 2).
Risk routing expected at build: payment (ledger, fees), proto (contract), auth (new RPC scopes, service-only ChargeSellerFee, anonymous RecordAdClick).
00-spec carries no NEEDS_HUMAN tag.

Sign-off (not spec-changing):
- DPO: 12-month retention of lapsed subscription rows (promotion-plan-billing).

Decisions that change a spec (default in brackets = what the change currently assumes):
1. Grandfathering of existing ACTIVE paid subscriptions (promotion-plan-billing): [one free 30-day term at deploy, no charge] or lapse to FREE at deploy.
2. Renewal on transient payment errors (promotion-plan-billing): [retry for SUBSCRIPTION_RENEWAL_RETRY_WINDOW=1h, then FREE; only FAILED_PRECONDITION lapses at once] or lapse on any error.
3. Scheduled downgrade (promotion-plan-billing, seller-plan-ui): [Subscribe(lower tier, FREE included) schedules the downgrade at period end; no proto field, so the UI cannot show "downgrade pending" after reload] or add pending_plan_id to the contract. Also: should the downgrade dialog state the lower plan's price and renewal debit?
4. Platform-voucher share on refunds (payment-settlement-commission): [never clawed back; seller keeps platform_discount_amount after a full refund] or reverse it too (new ledger movement).
5. Incoherent price breakdown: team-order [publishes old-shape event, bumps order_paid_breakdown_skipped_total, PAID not blocked]; team-payment [present-but-inconsistent breakdown is a permanent error parked on the DLQ, seller not credited until replay] or credit the paid amount without commission. Confirm both ends together.
6. Empty category on search (promotion-ads-serving, buyer-sponsored-slots): [keyword-only search returns no slots] or fall back to any active campaign. Also [slots hidden when organic search returns zero items].
7. Placement keys and unknown tokens (monetization-contract, promotion-ads-serving, buyer-sponsored-slots): [placement accepts exactly `search` and `home`; unknown placement error to be named in the contract comment]; RecordAdClick with an unknown/forged token: [NOT_FOUND] vs charged=false; expired/replayed/paused tokens [charged=false with listing_id]; self-click [token left unconsumed, no charge]; unpayable campaign [rollback, token stays re-clickable until expiry].
8. Budget rules (promotion-ads-serving, seller-ads-ui): [CreateAdCampaign rejects budget < bid]; [migration sets existing campaigns with bid < 500 or budget < bid to ENDED] or leave them ACTIVE but unserved; legacy listing_ids path [status-only, up to 50 ids by bid] or apply budget - spent >= bid too.
9. Campaign lifecycle (promotion-ads-campaign-mgmt, seller-ads-ui): [no top-up/edit-budget or end RPC in v1, so PAUSED_BUDGET is terminal; resume disabled in UI]; Pause on PAUSED_BUDGET [becomes PAUSED] or stays PAUSED_BUDGET; [owner-only, no admin access to list/pause/resume].
10. ChargeSellerFee semantics (payment-seller-fee-charge, monetization-contract): [reference_id reuse with a different seller or amount returns ALREADY_EXISTS, to be added to the contract comment; reference_id 1..64 chars; opaque, so plan-billing's sub:<id>:<period_seq+1>:<plan_id> key is accepted]; [only service-principal Go integration tests, one gateway-not-exposed e2e] or let platform-e2e call team-payment gRPC with a service principal.
11. Listing/featured caps (listing-plan-cap): [UpsertStorefront enforces featured_listings with reason PLAN_FEATURED_LIMIT, a name not in 00-spec] or drop the storefront part; [FREE featured_listings = 0, so FREE sellers feature nothing]; [missing/non-integer entitlement key fails closed as UNAVAILABLE]; [2 s entitlement timeout total across retries, per-seller advisory lock].
12. Seller UI wording and mapping (seller-plan-ui, seller-fee-breakdown-ui): [button labels change from "Đăng ký" to "Nâng cấp" / "Hạ gói", frontend/ui_seller_orders_money.feature updated]; [Subscribe FAILED_PRECONDITION shown as insufficient balance, so plan-billing must not reuse it for other refusals]; [gross card = ORDER_SETTLEMENT credit, label "Thực nhận trước phí"]; [unknown ledger types fall back to the raw string]; rate shown with decimals for non-integer percent?
13. Anonymous click route (buyer-sponsored-slots): with an expired buyer session, [forward the bearer token] or retry anonymously so the 204-always rule holds.
14. RecordAdClick rate limit (promotion-ads-serving): [inherit the gateway edge default] or state a per-client number.

Assumptions locked in unless answered (build verifies):
- monetization-contract merges first; proto RPCs/fields (OrderPaidEvent 9-12, GetOrderSettlement, ChargeSellerFee, COMMISSION/SUBSCRIPTION_FEE types, SponsoredSlot, RecordAdClick, ListMyAdCampaigns, Pause/Resume, PAUSED_BUDGET, GetMySubscription, CancelSubscription, term_days) are absent from the worktree today. Consumer copy pair table covers 35 vendored copies; it must grow if a slice adds a repo.
- Plan.price comment-only change in the contract; catalog price fix (199000 / 499000) lives in promotion-plan-billing.
- promotion-ads-serving lands the spent/impressions/clicks columns and PAUSED_BUDGET before promotion-ads-campaign-mgmt.
- team-gateway exposes GetOrderSettlement to seller principals, forwards Connect ErrorInfo details unchanged (PLAN_LISTING_LIMIT), and RecordAdClick needs no new scope.
- Ledger types reach the seller UI as bare strings (ORDER_SETTLEMENT, COMMISSION, ...).
- ChargeSellerFee scope name must match between payment-seller-fee-charge and promotion-plan-billing's upstream client.
- "search page 1 only" is enforced by the buyer frontend; admin-created campaigns have empty category and serve on home only.
- The ads listing title is joined client-side from getAllListings; a missing listing shows a fallback label.

Minor spec polish (non-blocking, fold in at build or a quick author round): order-paid-price-breakdown (negative platform_discount_amount scenario; move outbox scenarios to their own requirement; NFR <1 ms needs a benchmark task or "informational"); payment-settlement-commission (held-sale balance wording 596000/296000; 5 ms benchmark needs machine class or median of 3); payment-seller-fee-charge (seller-differs ALREADY_EXISTS scenario; "exactly three succeed"); promotion-plan-billing (Subscribe(FREE) no-op scenario, period_end == now boundary, placeholder row not ACTIVE, overdue metric scenario, sweeper has no RPC surface); promotion-ads-campaign-mgmt (empty/unknown id, UNAUTHENTICATED, malformed cursor, gateway pass-through, split pause/resume scenario, exact impressions figure); listing-plan-cap (malformed featured_listings key scenario); seller-ads-ui (pause/resume UNAVAILABLE scenario).
