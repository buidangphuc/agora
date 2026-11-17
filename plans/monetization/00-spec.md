# 00-spec — monetization: commission, paid seller plans, sponsored ads

initiative: monetization

## Goal
The platform earns on three levers, all as mock-money bookkeeping in integer VND (AGENTS.md §7):
every online-paid order books a platform commission against the seller's wallet and shows the
seller gross / fee / net; seller plans cost money for a fixed term and their caps are enforced; and
sponsored listings are shown to buyers on search and home, each charged click burning the campaign
budget, with campaigns the seller can list, pause and resume.

## Current state (verified on origin/main 06c44c17)
- Q13 commission — **missing.** Settlement credits the seller the full payment amount
  (team-payment/internal/store/settlement.go:289-295, `Amount: t.Amount` at :292); ledger types are only
  ORDER_SETTLEMENT / PAYOUT / REFUND_DEDUCTION (team-payment/internal/store/ledger.go:15-19,
  migrations/0006_wallet_ledger_integrity.up.sql type/sign CHECK). No commission/fee anywhere in
  Go/TS/Py/proto/SQL. `OrderPaidEvent` carries only `total_amount`
  (platform-core/packages/proto/platform/order/v1/order.proto:554-563). Side finding: the paid
  amount is `items_subtotal + shipping_fee - discount_amount` and `discount_amount` includes
  `platform_discount_amount` (order.proto:404-428), so **the seller funds the platform voucher today.**
- Q17 plans — **partial.** Catalog and entitlements exist (team-promotion/internal/store/subscription.go:63-115,
  GetEntitlements at internal/subscription/subscription.go:66); nothing enforces them
  (team-domain/internal/rpc/listing.go:180-197 CreateListing has no cap; no caller of
  `max_listings` / `featured_listings`). `Subscribe` is free and permanent (subscription.go:37-61,
  no term column in migrations/0002_subscriptions.up.sql). Plan prices are stored as 19 900 000 /
  49 900 000 with a "minor units" comment, so the seller UI shows ₫19.900.000 for PRO
  (fe-platform/packages/ui/src/format.ts:2 formats integer VND).
- Q14 ads — **partial (seller create only).** `CreateAdCampaign` with ownership check and caps
  (team-promotion/internal/sponsored/sponsored.go:70-121); `ListSponsoredSlots` ignores context and
  returns top 50 active by bid (sponsored.go:128, store/sponsored.go:124-128); budget never moves; no
  list/pause/resume RPC (promotion.proto:457-462). Buyer side: `listSponsoredSlots` wrappers exist
  (team-frontend/src/lib/gateway/promotion.ts:288, team-frontend-seller/src/lib/gateway/promotion.ts:408)
  but no page calls them. Seller campaign table is client state (team-frontend-seller/src/features/seller/AdCampaignForm.tsx:73).

## User stories
1. **Commission at settlement.** Given order O paid online with items_subtotal 300 000, shipping
   30 000, shop-voucher 20 000, when team-payment settles O, then the seller's ledger has
   ORDER_SETTLEMENT and one COMMISSION entry of −round(base × rate) in the same transaction, and the
   seller order page shows gross, commission and net.
   Edge: a redelivered OrderPaidEvent writes no second COMMISSION.
   Edge: a partial return refund of 40% gives back commission so that the cumulative reversal is
   round(commission × refunded / paid); a full refund gives back exactly the commission.
   Commission rate is flat 5% (500 bps) for every seller and category, read from `COMMISSION_RATE_BPS`; there is no per-category table in v1.
   Commission base is items_subtotal − shop_discount_amount (seller-funded voucher); shipping and platform_discount_amount are excluded. In the example: base = 300 000 − 20 000 = 280 000, commission = 14 000.
   The platform funds the platform voucher: the seller's ORDER_SETTLEMENT credit is the amount the buyer paid plus platform_discount_amount (today it is only the amount paid, so the seller bears the platform voucher). This MODIFIES the existing `seller-settlement-credit` requirement, so the payment slice's OpenSpec change must carry a `## MODIFIED Requirements` delta for it, restating the credit as paid + platform_discount_amount; events without the breakdown keep the old credit (paid only).
   Commission = floor((base × rate_bps + 5000) / 10000), i.e. half-up to whole VND, computed once per order on the full base. The reversal is cumulative: target = round_half_up(commission × refunded / paid) and each refund books target − already reversed, so the sum of reversals never exceeds the commission and a full refund returns exactly the commission.
   Commission is booked at settlement, when team-payment handles OrderPaidEvent, in the same DB transaction as the ORDER_SETTLEMENT credit (not at order completion).
   Commission is returned on refunds proportionally (COMMISSION_REVERSAL as above); the platform does not keep commission on refunded amounts.
2. **Paid plan with a term.** Given seller S with withdrawable wallet balance ≥ PRO price, when S
   subscribes to PRO, then a SUBSCRIPTION_FEE debit is booked once, the subscription is ACTIVE with
   `current_period_end = now + term`, and entitlements return PRO.
   Edge: insufficient withdrawable balance → FAILED_PRECONDITION, plan unchanged, nothing booked.
   Edge: at period end the sweeper renews (one fee per period, keyed by period) or, if the charge
   fails or S cancelled, sets S to FREE.
   Plan prices are FREE 0, PRO 199 000, PREMIUM 499 000 VND, each for a 30-day term (term_days = 30), stored as integer VND; this replaces the catalog values that display 100× too low.
   A seller pays for a plan by a SUBSCRIPTION_FEE debit from the wallet withdrawable balance via `ChargeSellerFee`; there is no card checkout.
   Upgrade takes effect immediately and charges the full new-plan price with no proration, starting a new 30-day period. Downgrade and cancel take effect at current_period_end (the current plan stays until then; cancel sets auto_renew = false). Auto-renew defaults to true; if the renewal charge fails the seller is set to FREE.
3. **Listing cap.** Given S on FREE (max_listings 10) owning 10 listings, when S creates an 11th,
   then CreateListing returns RESOURCE_EXHAUSTED with reason `PLAN_LISTING_LIMIT` and the seller
   form shows an upgrade link to /seller/plans.
   Edge: a seller downgraded while over the cap keeps existing listings; only new creates are blocked.
   Edge: team-promotion unavailable → CreateListing fails closed with UNAVAILABLE.
   All of the seller's listings count toward max_listings, whatever their status. When team-promotion is unavailable CreateListing fails closed with UNAVAILABLE; existing listings are never removed or hidden.
   featured_listings is a cap on the number of storefront featured_listing_ids; the plan cap overrides the fixed limit of 6 from the storefront cluster (S2). It carries no ad credit.
4. **Sponsored slot shown and charged.** Given an ACTIVE campaign C for listing L (bid B, budget
   remaining ≥ B), when a buyer searches in L's category, then the results page shows L in a slot
   labelled "Tài trợ"; when the buyer clicks it, then C is charged B once and the click navigates to L.
   Edge: a second click on the same impression token, an expired token, or a click by C's own
   seller charges nothing. Edge: when remaining budget < bid, C becomes PAUSED_BUDGET and is no
   longer served.
   Pricing is CPC, first-price: a click charges exactly the campaign's bid, and the minimum bid is 500 VND (a lower bid is INVALID_ARGUMENT).
   The ad budget is a bookkeeping counter on the campaign only in v1; it is not debited from the wallet and no AD_SPEND ledger entry is written.
   Placements are search page 1 with 2 sponsored slots and the home feed with 2 sponsored slots; a sponsored listing that also appears in the organic results of the same page is shown only as the sponsored slot and removed from the organic list (no duplicate).
   Relevance in v1 is category match plus bid ranking (highest bid first, ties by oldest campaign); there is no CTR model.
5. **Campaign management.** Given S has campaigns, when S opens /seller/ads, then S sees each
   campaign (listing, bid, budget, spent, impressions, clicks, status) after reload and can pause
   and resume it; another seller's campaign returns NOT_FOUND.

## Non-goals
- No real money, invoices, VAT/PIT withholding, or bank movement; all amounts are ledger bookkeeping.
- No commission on COD orders in v1 (they never settle into the wallet); this is a known gap, accepted.
- No commission discount as a plan perk. No backfill of commission on orders paid before deploy.
- No ad CTR model, no keyword bidding, no second-price auction, no ROAS / order attribution report.
- No sponsored items inside team-search or team-ai ranking; organic ranking is untouched.
- No gating of seller analytics by plan (per-listing analytics does not exist yet; Q8 is another cluster).
- No admin take-rate dashboard; fees live in the team-payment ledger, `order_facts` stays the only
  GMV/revenue source and is unchanged.
- No staging/prod deploy work (gitops values, Vault) — local first.

## Architecture decisions
- OrderPaidEvent gains the price breakdown (subtotal, shipping, discounts) set by team-order; team-payment
  computes commission from the event, no synchronous GetOrder. Why: the consumer stays async and replayable.
  An event without the breakdown (published before deploy) books no commission and is counted in a metric.
- Commission and its reversals are new ledger types written in the same DB transaction as the
  settlement credit / refund deduction, unique on (type, reference_id): COMMISSION references the
  payment id, COMMISSION_REVERSAL references the refund key. Reversal = cumulative target minus
  already reversed, so the sum never exceeds the commission. Why: mirrors seller-refund-deduction D3.
- The payout holdback holds a settlement net of its commission. Why: holding gross would block
  withdrawable money the seller really has.
- The rate is team-payment config (`COMMISSION_RATE_BPS`, integer basis points, validated 0..3000);
  a per-category table is only added if the user picks per-category. Why: one knob, no new store.
- Seller fees (plans only; ad budget is a counter in v1) are booked by a new service-to-service
  `PaymentService.ChargeSellerFee`, idempotent on `reference_id`, debiting only the withdrawable
  balance (same rule as payouts). team-gateway does not forward it. Why: one owner of the wallet.
- team-promotion owns plan terms (new columns, renewal in the existing sweeper loop) and calls
  team-payment via new `UPSTREAM_PAYMENT_ADDR`. team-domain calls GetEntitlements via new
  `UPSTREAM_PROMOTION_ADDR` and counts the seller's listings in its own store. No `*_ENABLED` flags:
  both upstreams are required config.
- Sponsored slots are merged by the buyer frontend server components (search page, home feed) from
  `ListSponsoredSlots`; team-promotion stores the listing's category on the campaign at create time,
  filters by the context category, ranks by bid, and issues one opaque impression token per served
  slot (table `ad_impressions`, no buyer id stored, purged after 30 days). `RecordAdClick(token)` is
  callable anonymously through the gateway and charges at most once per token.
- Plan prices are corrected to integer VND in a team-promotion migration (catalog + seed).

## Contracts
```proto
// platform/order/v1/order.proto — OrderPaidEvent (existing fields 1-8 unchanged)
optional int64 items_subtotal = 9;
optional int64 shipping_fee = 10;
optional int64 shop_discount_amount = 11;      // seller-funded voucher share
optional int64 platform_discount_amount = 12;  // platform voucher share

// platform/payment/v1/payment.proto — WalletTransactionType
WALLET_TRANSACTION_TYPE_COMMISSION = 4;
WALLET_TRANSACTION_TYPE_COMMISSION_REVERSAL = 5;
WALLET_TRANSACTION_TYPE_SUBSCRIPTION_FEE = 6;
WALLET_TRANSACTION_TYPE_AD_SPEND = 7;          // reserved, not written in v1 (ad budget is a counter)

// platform/payment/v1/payment.proto — add to PaymentService
// The order's seller or an admin; NOT_FOUND for anyone else and for an unsettled order.
rpc GetOrderSettlement(GetOrderSettlementRequest) returns (GetOrderSettlementResponse);
// Service-to-service only (team-promotion); team-gateway does not expose it.
rpc ChargeSellerFee(ChargeSellerFeeRequest) returns (ChargeSellerFeeResponse);

message GetOrderSettlementRequest { string order_id = 1; }
message GetOrderSettlementResponse {
  string order_id = 1;
  int64 credited = 2;             // ORDER_SETTLEMENT amount
  int64 commission = 3;           // positive VND, 0 when none
  int64 commission_reversed = 4;  // positive VND returned by refunds
  int64 refund_deducted = 5;      // positive VND
  int64 net = 6;                  // credited - commission + commission_reversed - refund_deducted
  int32 commission_rate_bps = 7;  // rate applied at settlement
}
enum SellerFeeKind {
  SELLER_FEE_KIND_UNSPECIFIED = 0;
  SELLER_FEE_KIND_SUBSCRIPTION = 1;
  SELLER_FEE_KIND_AD_SPEND = 2;  // reserved, rejected in v1
}
message ChargeSellerFeeRequest {
  string seller_id = 1;
  int64 amount = 2;               // > 0 VND, else INVALID_ARGUMENT
  SellerFeeKind kind = 3;
  string reference_id = 4;        // idempotency key, e.g. sub:<subscription_id>:<period_start>
}
message ChargeSellerFeeResponse {
  string entry_id = 1;
  bool created = 2;               // false = replay of reference_id, nothing new booked
}
// FAILED_PRECONDITION when amount exceeds the withdrawable balance (nothing booked).

// platform/promotion/v1/promotion.proto — Plan / SellerSubscription
// Plan: int32 term_days = 5;
// SellerSubscription: google.protobuf.Timestamp current_period_end = 8; bool auto_renew = 9;
// SubscriptionService:
rpc GetMySubscription(GetMySubscriptionRequest) returns (GetMySubscriptionResponse);
rpc CancelSubscription(CancelSubscriptionRequest) returns (CancelSubscriptionResponse);
message GetMySubscriptionRequest {}
message GetMySubscriptionResponse { SellerSubscription subscription = 1; } // FREE has none
message CancelSubscriptionRequest {}  // stops auto-renew; plan stays until current_period_end
message CancelSubscriptionResponse { SellerSubscription subscription = 1; }

// platform/promotion/v1/promotion.proto — AdCampaign / AdCampaignStatus / SponsoredService
// AdCampaignStatus: AD_CAMPAIGN_STATUS_PAUSED_BUDGET = 4;
// AdCampaign: int64 spent = 8; int64 impressions = 9; int64 clicks = 10; string category_id = 11;
// ListSponsoredSlotsRequest: string placement = 2; string category_id = 3; int32 limit = 4; // 1..4
message SponsoredSlot {
  string listing_id = 1;
  string campaign_id = 2;
  string impression_token = 3;    // opaque, single-use for billing, expires after 30 min
}
// ListSponsoredSlotsResponse: repeated SponsoredSlot slots = 2; (listing_ids kept for old callers)
rpc ListMyAdCampaigns(ListMyAdCampaignsRequest) returns (ListMyAdCampaignsResponse);
rpc PauseAdCampaign(PauseAdCampaignRequest) returns (PauseAdCampaignResponse);
rpc ResumeAdCampaign(ResumeAdCampaignRequest) returns (ResumeAdCampaignResponse);
rpc RecordAdClick(RecordAdClickRequest) returns (RecordAdClickResponse);
message ListMyAdCampaignsRequest { platform.common.v1.PageRequest page = 1; }
message ListMyAdCampaignsResponse { repeated AdCampaign campaigns = 1; platform.common.v1.PageResponse page = 2; }
message PauseAdCampaignRequest { string campaign_id = 1; }
message PauseAdCampaignResponse { AdCampaign campaign = 1; }
message ResumeAdCampaignRequest { string campaign_id = 1; }   // FAILED_PRECONDITION if budget < bid
message ResumeAdCampaignResponse { AdCampaign campaign = 1; }
message RecordAdClickRequest { string impression_token = 1; }
message RecordAdClickResponse { string listing_id = 1; bool charged = 2; }
```
Existing contracts consumed unchanged: `order.events/OrderPaidEvent` (extended above),
`order.events/ReturnRefunded`, `order.events/OrderCancelled`, `SubscriptionService.GetEntitlements`,
`ListingService.GetListing`, `SponsoredService.CreateAdCampaign`.

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| contracts | monetization-contract | platform-core | – | provides: all RPCs/fields above | platform-core |
| paid order carries its price breakdown | order-paid-price-breakdown | team-order | monetization-contract | uses: order.events/OrderPaidEvent | team-order, platform-e2e |
| commission booked and reversed in the ledger | payment-settlement-commission | team-payment | order-paid-price-breakdown | provides impl: PaymentService.GetOrderSettlement | team-payment, team-gateway, platform-e2e |
| seller fee charged from the wallet | payment-seller-fee-charge | team-payment | monetization-contract | provides impl: PaymentService.ChargeSellerFee | team-payment, platform-e2e |
| seller sees gross / fee / net | seller-fee-breakdown-ui | team-frontend-seller | payment-settlement-commission | uses: PaymentService.GetOrderSettlement | team-frontend-seller, platform-e2e |
| plans are paid, termed and renewed | promotion-plan-billing | team-promotion | payment-seller-fee-charge | uses: PaymentService.ChargeSellerFee; provides impl: SubscriptionService.GetMySubscription, CancelSubscription | team-promotion, team-gateway, docker-compose, platform-e2e |
| listing cap enforced by plan | listing-plan-cap | team-domain | monetization-contract | uses: SubscriptionService.GetEntitlements | team-domain, docker-compose, platform-e2e |
| seller plan page and cap CTA | seller-plan-ui | team-frontend-seller | promotion-plan-billing, listing-plan-cap | uses: SubscriptionService.GetMySubscription, CancelSubscription | team-frontend-seller, platform-e2e |
| sponsored slots served, clicks charged | promotion-ads-serving | team-promotion | monetization-contract | provides impl: SponsoredService.RecordAdClick, ListSponsoredSlots | team-promotion, team-gateway, platform-e2e |
| seller manages campaigns | promotion-ads-campaign-mgmt | team-promotion | promotion-ads-serving | provides impl: SponsoredService.ListMyAdCampaigns, PauseAdCampaign, ResumeAdCampaign | team-promotion, team-gateway, platform-e2e |
| buyer sees "Tài trợ" slots | buyer-sponsored-slots | team-frontend | promotion-ads-serving | uses: SponsoredService.ListSponsoredSlots, RecordAdClick | team-frontend, platform-e2e |
| seller ads page lists, pauses, resumes | seller-ads-ui | team-frontend-seller | promotion-ads-campaign-mgmt | uses: SponsoredService.ListMyAdCampaigns, PauseAdCampaign, ResumeAdCampaign | team-frontend-seller, platform-e2e |

Hot files edited by more than one slice (initiative-build serialises those slices):
- team-payment/internal/store/ledger.go, team-payment/migrations/ (type/sign CHECK) — payment-settlement-commission, payment-seller-fee-charge.
- team-promotion/internal/store/migrations/ (numbering), internal/sponsored/sponsored.go, internal/store/sponsored.go — promotion-plan-billing, promotion-ads-serving, promotion-ads-campaign-mgmt.
- team-gateway/internal/edge/promotion.go — promotion-plan-billing, promotion-ads-serving, promotion-ads-campaign-mgmt; edge/payment.go — payment-settlement-commission.
- docker-compose.yaml (team-promotion UPSTREAM_PAYMENT_ADDR, team-domain UPSTREAM_PROMOTION_ADDR) — promotion-plan-billing, listing-plan-cap.
- team-frontend-seller/src/lib/gateway/promotion.ts — seller-plan-ui, seller-ads-ui; lib/gateway/payment.ts — seller-fee-breakdown-ui.

## Out of initiative
- Wallet page shows raw ledger type strings (team-frontend-seller/src/app/seller/wallet/page.tsx:33);
  Vietnamese labels for all types are a task inside seller-fee-breakdown-ui, not a slice.
- Plan price correction to integer VND rides inside promotion-plan-billing.

## NFR and data
- NFR: commission adds ≤ 5 ms p95 to settlement (same transaction). `ListSponsoredSlots` p95 ≤ 30 ms
  locally for limit ≤ 4; the buyer page renders organic results if it errors or exceeds 150 ms
  (slots dropped, never blocks). `RecordAdClick` idempotent per token; the click charge and the
  budget update are one compare-and-set (no overspend under concurrent clicks). Ledger reconciles:
  balance = Σ entries, zero drift, after any order of credit / refund / commission events.
- Money: integer VND everywhere, int64, no fractional units; rates in integer basis points.
- Personal data: seller id on ledger and subscription rows (existing purpose: payouts/billing).
  `ad_impressions` stores no buyer id, IP or session; self-click check uses the forwarded principal
  in memory only. Never log raw seller/buyer ids (sloglint forbidden-keys).
- Risk routing expected: `payment` (ledger, fees), `proto` (contract), `auth` (new RPC scopes,
  service-only ChargeSellerFee, anonymous RecordAdClick).

## Conventions
- team-payment: mirror `CreditSettlement` / `RefundDeduction` and `insertLedgerOnce`
  (internal/store/settlement.go, ledger.go); new migration 0018 extends the type/sign CHECK.
  Docker-backed tests need `GO_SDK_REQUIRE_DOCKER=1`.
- team-order: OrderPaidEvent is built in team-order/internal/events/publisher.go:66.
- team-promotion: renewal and impression purge go into the existing sweeper loop
  (internal/consumer/sweeper.go); campaign ownership via internal/listing/owner.go.
- team-domain: entitlement lookup behind a small port in internal/listing; RESOURCE_EXHAUSTED with
  `errdetails.ErrorInfo{Reason: "PLAN_LISTING_LIMIT"}`.
- Frontends: seller UI lives in team-frontend-seller (not team-frontend); buyer UI in team-frontend;
  use fe-platform `formatPrice` (integer VND). Mock data via the real service's guarded mode, no
  fake containers or route fulfils in e2e.

## Locked
2026-11-13 — Confirmed by Bùi Đăng Phúc (all defaults accepted). Flat 5% commission (500 bps) on items_subtotal − shop voucher, half-up per order, booked at settlement and returned proportionally on refunds; platform funds the platform voucher (MODIFIES seller-settlement-credit); COD carries no commission; PRO 199 000 / PREMIUM 499 000 VND per 30 days paid from the wallet, upgrade immediate, downgrade/cancel at period end, auto-renew, failed renew → FREE; all listings count toward max_listings, fail closed; featured_listings caps featured ids; CPC first-price, min bid 500 VND, budget is a counter, 2 slots on search page 1 and home, deduped, category + bid ranking.
