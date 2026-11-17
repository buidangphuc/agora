## Why

The monetization initiative (commission, paid seller plans, sponsored ads) needs one set of additive proto changes before eleven consumer slices can build in parallel. The price breakdown on `OrderPaidEvent`, the new ledger types, `GetOrderSettlement`, `ChargeSellerFee`, plan terms with `GetMySubscription` and `CancelSubscription`, and the ad serving, click and campaign management RPCs are all owned by platform-core. Shipping them first, with vendored consumer copies refreshed, unblocks `order-paid-price-breakdown`, `payment-seller-fee-charge`, `listing-plan-cap` and `promotion-ads-serving` and, through them, the rest.

initiative: monetization

## What Changes

All edits are in `platform-core/packages/proto/platform`; no field is renamed, renumbered or removed.

- `order/v1/order.proto`: `OrderPaidEvent` gains `optional int64 items_subtotal = 9`, `shipping_fee = 10`, `shop_discount_amount = 11`, `platform_discount_amount = 12`. Optional presence lets a consumer tell an event published before deploy (no breakdown) from a real zero.
- `payment/v1/payment.proto`: `WalletTransactionType` gains `COMMISSION = 4`, `COMMISSION_REVERSAL = 5`, `SUBSCRIPTION_FEE = 6`, `AD_SPEND = 7` (reserved, not written in v1). `PaymentService` gains `GetOrderSettlement` (order's seller or admin) and `ChargeSellerFee` (service-to-service only), with `SellerFeeKind` (`SUBSCRIPTION = 1`, `AD_SPEND = 2` reserved and rejected in v1) and the request and response messages exactly as in `plans/monetization/00-spec.md` Contracts.
- `promotion/v1/promotion.proto`: `Plan.term_days = 5`; `SellerSubscription.current_period_end = 8` and `auto_renew = 9`; `SubscriptionService` gains `GetMySubscription` and `CancelSubscription`; the `Plan.price` comment changes from "minor units" to integer VND per term. `AdCampaignStatus` gains `PAUSED_BUDGET = 4`; `AdCampaign` gains `spent = 8`, `impressions = 9`, `clicks = 10`, `category_id = 11`; `ListSponsoredSlotsRequest` gains `placement = 2`, `category_id = 3`, `limit = 4` (1..4); new `SponsoredSlot { listing_id = 1, campaign_id = 2, impression_token = 3 }`; `ListSponsoredSlotsResponse` gains `repeated SponsoredSlot slots = 2` (`listing_ids = 1` kept); `SponsoredService` gains `ListMyAdCampaigns`, `PauseAdCampaign`, `ResumeAdCampaign`, `RecordAdClick`.
- Proto comments document the semantics that tests pin: integer VND everywhere, the `ChargeSellerFee` idempotency and FAILED_PRECONDITION rule, the `GetOrderSettlement` net formula and NOT_FOUND rule, the 1..4 limit, token single use and 30 minute expiry, anonymous callability of `RecordAdClick`, and that `ChargeSellerFee` is not exposed by team-gateway.
- Baseline for every compatibility check: the enum, message and rpc tables of the three files, pinned in the test files, copied from `origin/main` at the time of this change.
- Vendored copies under `<repo>/proto/platform/...` are refreshed byte-for-byte in every consumer repo that already holds the file: order.proto (team-analytics, team-chat, team-domain, team-engagement, team-frontend, team-frontend-admin, team-frontend-seller, team-gateway, team-notification, team-order, team-payment, team-promotion, team-referral, team-search), payment.proto (the same 14 repos) and promotion.proto (team-analytics, team-frontend, team-frontend-admin, team-frontend-seller, team-gateway, team-order, team-promotion). Checked-in stubs are regenerated with each repo's own `buf generate`.

## Capabilities

### New Capabilities
- `monetization-contract`: the proto contract for the order price breakdown, commission and fee ledger types, settlement and fee RPCs, plan terms and cancellation, and sponsored ad serving, click and campaign management.

### Modified Capabilities
none

## Impact

- `platform-core/packages/proto/platform/{order,payment,promotion}/v1/*.proto`, new `monetization_compat_test.go` beside each, and `monetization_consumer_copies_test.sh`.
- Vendored `proto/` copies and regenerated stubs in the consumer repos listed above (copy refresh only, no behaviour change).
- team-gateway gets the refreshed copies; routes and auth scopes for the new RPCs belong to the consumer slices, and `ChargeSellerFee` gets no route.

## Non-goals

Implementing any RPC or handler, computing commission, booking ledger entries, gateway routes and auth scopes, frontends. Any change to the existing fields of `OrderPaidEvent`, `WalletEntry` (its `type` stays a string) or `ListSponsoredSlotsResponse.listing_ids`. Writing `AD_SPEND` entries or accepting `SELLER_FEE_KIND_AD_SPEND` (reserved). Per-category commission.

## Non-functional requirements

- `buf lint` and `buf breaking` (FILE) against `origin/main` pass: purely additive.
- Wire compatibility: a payload from the previous schema parses with the new one and the reverse, existing fields unchanged. A consumer meeting `WALLET_TRANSACTION_TYPE` 4 to 7 or `AD_CAMPAIGN_STATUS` 4 before it is updated reads an unknown enum number and must not crash.
- Money: every amount is int64 integer VND; rates are integer basis points; no float field.
- Availability: no runtime component; rollout is proto first, consumers after. Latency budgets (settlement commission at most 5 ms p95, `ListSponsoredSlots` at most 30 ms p95) are met and tested in the consumer slices.

## Data classification

| field | personal data | purpose | consent basis | retention |
|---|---|---|---|---|
| `OrderPaidEvent` breakdown amounts | no: order amounts, linked to an order that already carries buyer and seller ids | let team-payment compute commission without a synchronous call | contract performance (existing order processing) | as the order event |
| `ChargeSellerFeeRequest.seller_id`, `amount`, `reference_id` | seller id: pseudonymous account id | bill a plan fee to the seller's wallet | contract performance (seller plan purchase) | as the wallet ledger |
| `GetOrderSettlementResponse` amounts | no: amounts for an order the caller owns | show the seller gross, fee and net | contract performance | not stored by the contract |
| `SellerSubscription.current_period_end`, `auto_renew` | no: plan term state of a seller id | term and renewal of a paid plan | contract performance | as the subscription row |
| `AdCampaign.spent`, `impressions`, `clicks`, `category_id` | no: campaign counters of a seller id | let the seller manage campaigns | contract performance | as the campaign row |
| `SponsoredSlot.impression_token`, `RecordAdClickRequest.impression_token` | no: opaque token carrying no buyer id, IP or session | bill a click at most once | legitimate interest (fraud-safe billing); no buyer identifier is stored | 30 days, set by the `promotion-ads-serving` slice |

No buyer id, IP address or session id is added to any new message.

## Threat model

- Assets: seller wallet balances (ledger), plan state, campaign budgets, sponsored ranking.
- Abuse cases: a caller reaching `ChargeSellerFee` through the edge to debit any seller; a seller reading another seller's settlement; a replayed or forged impression token inflating clicks; a seller clicking their own ad; a negative or zero fee amount; an oversized `limit` on `ListSponsoredSlots`.
- Authorisation rules: `ChargeSellerFee` is service-to-service only and team-gateway does not route it; `GetOrderSettlement` is the order's seller or an admin and returns NOT_FOUND to anyone else; `ListMyAdCampaigns`, `PauseAdCampaign`, `ResumeAdCampaign`, `GetMySubscription` and `CancelSubscription` act on the caller's own seller id; `RecordAdClick` is anonymous and the token is the only credential. The contract documents these rules; enforcement and tests live in the consumer slices.
