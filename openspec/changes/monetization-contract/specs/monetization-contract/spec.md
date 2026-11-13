## ADDED Requirements

### Requirement: Order paid event carries its price breakdown
`OrderPaidEvent` SHALL add `optional int64 items_subtotal = 9`, `optional int64 shipping_fee = 10`, `optional int64 shop_discount_amount = 11` and `optional int64 platform_discount_amount = 12`, documented as integer VND, with `shop_discount_amount` the seller-funded voucher share and `platform_discount_amount` the platform voucher share. Fields 1 to 8 SHALL keep their names and numbers.

#### Scenario: Breakdown fields present at 9 to 12
- **WHEN** the descriptor of `platform.order.v1.OrderPaidEvent` is inspected
- **THEN** it has `items_subtotal=9`, `shipping_fee=10`, `shop_discount_amount=11` and `platform_discount_amount=12`, each `optional int64`, and fields 1 to 8 are identical to the baseline pinned in the test
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/monetization_compat_test.go` "TestMonetization_OrderPaidBreakdownShape"

#### Scenario: Missing breakdown is distinct from zero
- **WHEN** an event with `shop_discount_amount` set to 0 and an event with it unset are serialised and parsed
- **THEN** the first reads presence true with value 0, the second reads presence false, and the bytes differ
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/monetization_compat_test.go` "TestMonetization_BreakdownPresenceRoundTrip"

#### Scenario: Event published before deploy still parses
- **WHEN** bytes of an `OrderPaidEvent` serialised by the previous schema are parsed with the new one
- **THEN** parsing succeeds, fields 1 to 8 are unchanged and the four breakdown fields read as absent
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/monetization_compat_test.go` "TestMonetization_OldOrderPaidParses"

#### Scenario: Breakdown semantics are documented
- **WHEN** the comments on the four new fields in order.proto are read
- **THEN** they state integer VND, that `shop_discount_amount` is the seller-funded voucher share and `platform_discount_amount` the platform voucher share, and that an absent breakdown means the event was published without one
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/monetization_compat_test.go` "TestMonetization_BreakdownDocumented"

### Requirement: Fee and commission ledger types
`WalletTransactionType` SHALL add `WALLET_TRANSACTION_TYPE_COMMISSION = 4`, `WALLET_TRANSACTION_TYPE_COMMISSION_REVERSAL = 5`, `WALLET_TRANSACTION_TYPE_SUBSCRIPTION_FEE = 6` and `WALLET_TRANSACTION_TYPE_AD_SPEND = 7`, the last documented as reserved and never written in v1. Values 0 to 3 SHALL keep their numbers.

#### Scenario: Four new types at 4 to 7
- **WHEN** the descriptor of `platform.payment.v1.WalletTransactionType` is inspected
- **THEN** it has `COMMISSION=4`, `COMMISSION_REVERSAL=5`, `SUBSCRIPTION_FEE=6` and `AD_SPEND=7`, exactly 8 values in total, and values 0 to 3 are identical to the baseline pinned in the test
- **VERIFIED BY**: `platform-core/packages/proto/platform/payment/v1/monetization_compat_test.go` "TestMonetization_WalletTypesShape"

#### Scenario: Old consumer meets a new type
- **WHEN** a message carrying a `WalletTransactionType` field set to 4 is parsed by the previous schema
- **THEN** parsing succeeds, the field reads as enum number 4 with no defined name and every other field is unchanged
- **VERIFIED BY**: `platform-core/packages/proto/platform/payment/v1/monetization_compat_test.go` "TestMonetization_OldSchemaParsesNewType"

#### Scenario: AD_SPEND is documented as reserved
- **WHEN** the comment on `WALLET_TRANSACTION_TYPE_AD_SPEND` is read
- **THEN** it states that the value is reserved and is not written in v1 because the ad budget is a campaign counter
- **VERIFIED BY**: `platform-core/packages/proto/platform/payment/v1/monetization_compat_test.go` "TestMonetization_AdSpendDocumentedReserved"

### Requirement: Order settlement lookup RPC
`PaymentService` SHALL add `rpc GetOrderSettlement(GetOrderSettlementRequest) returns (GetOrderSettlementResponse)` with `GetOrderSettlementRequest { string order_id = 1; }` and `GetOrderSettlementResponse { string order_id = 1; int64 credited = 2; int64 commission = 3; int64 commission_reversed = 4; int64 refund_deducted = 5; int64 net = 6; int32 commission_rate_bps = 7; }`. The comments SHALL state that all amounts are positive integer VND, that `net = credited - commission + commission_reversed - refund_deducted`, that the caller must be the order's seller or an admin, and that NOT_FOUND is returned to anyone else and for an unsettled order.

#### Scenario: RPC and messages present at the specified numbers
- **WHEN** the descriptors of `PaymentService`, `GetOrderSettlementRequest` and `GetOrderSettlementResponse` are inspected
- **THEN** the service has the `GetOrderSettlement` method from request to response type, the request has `order_id=1 string`, and the response has fields 1 to 7 with the names and types listed above
- **VERIFIED BY**: `platform-core/packages/proto/platform/payment/v1/monetization_compat_test.go` "TestMonetization_GetOrderSettlementShape"

#### Scenario: Net formula and access rule are documented
- **WHEN** the comments on the `GetOrderSettlement` rpc and on `net` are read
- **THEN** they contain the formula `credited - commission + commission_reversed - refund_deducted`, state seller or admin access, and state NOT_FOUND for another seller's order and for an unsettled order
- **VERIFIED BY**: `platform-core/packages/proto/platform/payment/v1/monetization_compat_test.go` "TestMonetization_SettlementSemanticsDocumented"

#### Scenario: Another seller and an unsettled order are denied as not found
- **WHEN** the comment on the `GetOrderSettlement` rpc is read for the caller who is not the order's seller or an admin and for an order with no settlement
- **THEN** it states that the permission rule returns NOT_FOUND in both cases, so the RPC does not reveal whether another seller's order exists
- **VERIFIED BY**: `platform-core/packages/proto/platform/payment/v1/monetization_compat_test.go` "TestMonetization_SettlementNotFoundDocumented"

#### Scenario: Amount fields are integer VND
- **WHEN** the response fields `credited`, `commission`, `commission_reversed`, `refund_deducted` and `net` are listed
- **THEN** every one has kind int64, and no float or double field exists in the two messages
- **VERIFIED BY**: `platform-core/packages/proto/platform/payment/v1/monetization_compat_test.go` "TestMonetization_NoFloatMoney"

### Requirement: Seller fee charge RPC
`PaymentService` SHALL add `rpc ChargeSellerFee(ChargeSellerFeeRequest) returns (ChargeSellerFeeResponse)` with `enum SellerFeeKind { SELLER_FEE_KIND_UNSPECIFIED = 0; SELLER_FEE_KIND_SUBSCRIPTION = 1; SELLER_FEE_KIND_AD_SPEND = 2; }`, `ChargeSellerFeeRequest { string seller_id = 1; int64 amount = 2; SellerFeeKind kind = 3; string reference_id = 4; }` and `ChargeSellerFeeResponse { string entry_id = 1; bool created = 2; }`. The comments SHALL state that the RPC is service-to-service only and not exposed by team-gateway, that `amount` is integer VND greater than 0 and any other value is INVALID_ARGUMENT, that `reference_id` is the idempotency key and a replay returns `created = false` with nothing booked, that only the withdrawable balance is debited and FAILED_PRECONDITION is returned with nothing booked when the amount exceeds it, and that `SELLER_FEE_KIND_AD_SPEND` is reserved and rejected in v1.

#### Scenario: RPC, enum and messages present at the specified numbers
- **WHEN** the descriptors of `PaymentService`, `SellerFeeKind`, `ChargeSellerFeeRequest` and `ChargeSellerFeeResponse` are inspected
- **THEN** the method exists from request to response type, the enum has values 0 to 2 with the names above, the request has fields 1 to 4 and the response has `entry_id=1 string` and `created=2 bool`
- **VERIFIED BY**: `platform-core/packages/proto/platform/payment/v1/monetization_compat_test.go` "TestMonetization_ChargeSellerFeeShape"

#### Scenario: Idempotency and error rules are documented
- **WHEN** the comments on the `ChargeSellerFee` rpc, `amount`, `reference_id` and `created` are read
- **THEN** they state that `amount` must be greater than 0 else INVALID_ARGUMENT, that a replayed `reference_id` returns `created` false with nothing booked, that only the withdrawable balance is debited, and that FAILED_PRECONDITION means nothing was booked
- **VERIFIED BY**: `platform-core/packages/proto/platform/payment/v1/monetization_compat_test.go` "TestMonetization_ChargeSellerFeeSemanticsDocumented"

#### Scenario: The RPC is documented as internal and ad spend as reserved
- **WHEN** the comments on the `ChargeSellerFee` rpc and on `SELLER_FEE_KIND_AD_SPEND` are read
- **THEN** the first states that the RPC is service-to-service only and not exposed by team-gateway, and the second that the value is reserved and rejected in v1
- **VERIFIED BY**: `platform-core/packages/proto/platform/payment/v1/monetization_compat_test.go` "TestMonetization_ChargeSellerFeeInternalOnlyDocumented"

#### Scenario: Fee charge is not reachable by a seller through the edge
- **WHEN** the comment on the `ChargeSellerFee` rpc is read for permission
- **THEN** it states that no end-user principal or scope may call it, so a seller cannot debit another seller's wallet
- **VERIFIED BY**: `platform-core/packages/proto/platform/payment/v1/monetization_compat_test.go` "TestMonetization_ChargeSellerFeeNoUserScopeDocumented"

#### Scenario: Existing payment surface is untouched
- **WHEN** the `PaymentService` methods and the messages that existed before this change are compared with the baseline pinned in the test
- **THEN** every earlier rpc, message, field name, number and type is identical, including `WalletEntry.type` as a string
- **VERIFIED BY**: `platform-core/packages/proto/platform/payment/v1/monetization_compat_test.go` "TestMonetization_ExistingPaymentSurfaceUntouched"

### Requirement: Plan terms and subscription reads
`Plan` SHALL add `int32 term_days = 5` and its `price` comment SHALL state integer VND per term. `SellerSubscription` SHALL add `google.protobuf.Timestamp current_period_end = 8` and `bool auto_renew = 9`. `SubscriptionService` SHALL add `rpc GetMySubscription(GetMySubscriptionRequest) returns (GetMySubscriptionResponse)` with `GetMySubscriptionResponse { SellerSubscription subscription = 1; }` (a FREE seller has none) and `rpc CancelSubscription(CancelSubscriptionRequest) returns (CancelSubscriptionResponse)` with `CancelSubscriptionResponse { SellerSubscription subscription = 1; }`. The requests are empty and act on the caller's own seller id; `CancelSubscription` stops auto-renew and the plan stays until `current_period_end`.

#### Scenario: Term and period fields present at the specified numbers
- **WHEN** the descriptors of `Plan` and `SellerSubscription` are inspected
- **THEN** `Plan` has `term_days=5 int32`, `SellerSubscription` has `current_period_end=8 Timestamp` and `auto_renew=9 bool`, and every earlier field is identical to the baseline pinned in the test
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/monetization_compat_test.go` "TestMonetization_PlanTermShape"

#### Scenario: Subscription RPCs present with empty requests
- **WHEN** the descriptors of `SubscriptionService`, `GetMySubscriptionRequest` and `CancelSubscriptionRequest` are inspected
- **THEN** `GetMySubscription` and `CancelSubscription` exist with their response types, both requests have no fields, and each response has `subscription=1 SellerSubscription`
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/monetization_compat_test.go` "TestMonetization_SubscriptionRpcShape"

#### Scenario: Price unit and cancel semantics are documented
- **WHEN** the comments on `Plan.price`, `GetMySubscriptionResponse.subscription` and the `CancelSubscription` rpc are read
- **THEN** they state integer VND per term, that a FREE seller has no subscription, that cancel stops auto-renew, and that the plan stays until `current_period_end`
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/monetization_compat_test.go` "TestMonetization_SubscriptionSemanticsDocumented"

#### Scenario: Subscription reads take no seller id
- **WHEN** the fields of `GetMySubscriptionRequest` and `CancelSubscriptionRequest` are listed
- **THEN** both lists are empty, so a caller cannot name another seller
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/monetization_compat_test.go` "TestMonetization_SubscriptionRequestsCarryNoSellerId"

### Requirement: Ad campaign counters and budget status
`AdCampaignStatus` SHALL add `AD_CAMPAIGN_STATUS_PAUSED_BUDGET = 4` and `AdCampaign` SHALL add `int64 spent = 8`, `int64 impressions = 9`, `int64 clicks = 10` and `string category_id = 11`. The comments SHALL state integer VND for `budget`, `bid` (minimum 500 VND, a lower value is INVALID_ARGUMENT) and `spent`, and that `PAUSED_BUDGET` means the remaining budget is below the bid.

#### Scenario: Status and counters present at the specified numbers
- **WHEN** the descriptors of `AdCampaignStatus` and `AdCampaign` are inspected
- **THEN** the enum has `PAUSED_BUDGET=4` and exactly 5 values, `AdCampaign` has `spent=8`, `impressions=9`, `clicks=10` int64 and `category_id=11` string, and every earlier value and field is identical to the baseline pinned in the test
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/monetization_compat_test.go` "TestMonetization_AdCampaignShape"

#### Scenario: Old consumer meets the new status
- **WHEN** a message carrying an `AdCampaignStatus` field set to 4 is parsed by the previous schema
- **THEN** parsing succeeds, the field reads as enum number 4 with no defined name and every other field is unchanged
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/monetization_compat_test.go` "TestMonetization_OldSchemaParsesPausedBudget"

#### Scenario: Minimum bid and budget status are documented
- **WHEN** the comments on `AdCampaign.bid`, `AdCampaign.spent` and `AD_CAMPAIGN_STATUS_PAUSED_BUDGET` are read
- **THEN** they state integer VND, a minimum bid of 500 VND with INVALID_ARGUMENT below it, and that the status means remaining budget below the bid
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/monetization_compat_test.go` "TestMonetization_AdCampaignSemanticsDocumented"

### Requirement: Sponsored slot serving contract
`ListSponsoredSlotsRequest` SHALL add `string placement = 2`, `string category_id = 3` and `int32 limit = 4`, documented as 1..4 with any other value INVALID_ARGUMENT, and keep `context_str = 1`. A new message `SponsoredSlot { string listing_id = 1; string campaign_id = 2; string impression_token = 3; }` SHALL be returned in `ListSponsoredSlotsResponse.slots = 2`, and `listing_ids = 1` SHALL stay for old callers. The comment on `impression_token` SHALL state that it is opaque, single-use for billing and expires after 30 minutes.

#### Scenario: Request and response fields present at the specified numbers
- **WHEN** the descriptors of `ListSponsoredSlotsRequest`, `SponsoredSlot` and `ListSponsoredSlotsResponse` are inspected
- **THEN** the request has `context_str=1`, `placement=2` string, `category_id=3` string and `limit=4` int32, `SponsoredSlot` has fields 1 to 3 string, and the response has `listing_ids=1` and `slots=2 repeated SponsoredSlot`
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/monetization_compat_test.go` "TestMonetization_SponsoredSlotsShape"

#### Scenario: Limit bound and token semantics are documented
- **WHEN** the comments on `limit` and `impression_token` are read
- **THEN** the first contains the text `1..4` and `INVALID_ARGUMENT`, and the second states opaque, single-use for billing and 30 minute expiry
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/monetization_compat_test.go` "TestMonetization_SlotBoundAndTokenDocumented"

#### Scenario: A request without the new fields still parses
- **WHEN** bytes of a `ListSponsoredSlotsRequest` and `ListSponsoredSlotsResponse` serialised by the previous schema are parsed with the new one
- **THEN** both parse, `context_str` and `listing_ids` are unchanged, and the new fields read as their zero values
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/monetization_compat_test.go` "TestMonetization_OldSponsoredSlotsParse"

#### Scenario: The slot carries no buyer identifier
- **WHEN** the fields of `SponsoredSlot` and `RecordAdClickRequest` are listed
- **THEN** they are exactly `listing_id`, `campaign_id`, `impression_token` and `impression_token`, with no buyer, user, session or IP field
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/monetization_compat_test.go` "TestMonetization_NoBuyerIdentifiers"

### Requirement: Ad click and campaign management RPCs
`SponsoredService` SHALL add `ListMyAdCampaigns`, `PauseAdCampaign`, `ResumeAdCampaign` and `RecordAdClick` with `ListMyAdCampaignsRequest { platform.common.v1.PageRequest page = 1; }`, `ListMyAdCampaignsResponse { repeated AdCampaign campaigns = 1; platform.common.v1.PageResponse page = 2; }`, `PauseAdCampaignRequest { string campaign_id = 1; }`, `PauseAdCampaignResponse { AdCampaign campaign = 1; }`, `ResumeAdCampaignRequest { string campaign_id = 1; }`, `ResumeAdCampaignResponse { AdCampaign campaign = 1; }`, `RecordAdClickRequest { string impression_token = 1; }` and `RecordAdClickResponse { string listing_id = 1; bool charged = 2; }`. The comments SHALL state that the four management RPCs act on the caller's own campaigns and return NOT_FOUND for another seller's campaign, that `ResumeAdCampaign` returns FAILED_PRECONDITION when budget is below the bid, and that `RecordAdClick` is callable anonymously, charges at most once per token, and returns `charged = false` for a replayed or expired token or the campaign's own seller.

#### Scenario: Four RPCs and their messages present at the specified numbers
- **WHEN** the descriptors of `SponsoredService` and the eight new messages are inspected
- **THEN** the four methods exist with their request and response types and every message has the fields and numbers listed above
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/monetization_compat_test.go` "TestMonetization_AdRpcsShape"

#### Scenario: Ownership and error rules are documented
- **WHEN** the comments on `PauseAdCampaign`, `ResumeAdCampaign` and `ListMyAdCampaigns` are read
- **THEN** they state that the caller's own campaigns only are visible, NOT_FOUND for another seller's campaign, and FAILED_PRECONDITION on resume when budget is below the bid
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/monetization_compat_test.go` "TestMonetization_CampaignRulesDocumented"

#### Scenario: Click rules are documented
- **WHEN** the comments on the `RecordAdClick` rpc and on `charged` are read
- **THEN** they state that the RPC is callable anonymously, that a click is charged at most once per token, and that `charged` is false for a replayed token, an expired token and a click by the campaign's own seller
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/monetization_compat_test.go` "TestMonetization_ClickRulesDocumented"

#### Scenario: Existing promotion surface is untouched
- **WHEN** the `SubscriptionService` and `SponsoredService` methods and the messages that existed before this change are compared with the baseline pinned in the test
- **THEN** every earlier rpc, message, field name, number and type is identical
- **VERIFIED BY**: `platform-core/packages/proto/platform/promotion/v1/monetization_compat_test.go` "TestMonetization_ExistingPromotionSurfaceUntouched"

### Requirement: Consumer copies stay identical
Every repo that vendors `order.proto`, `payment.proto` or `promotion.proto` under `proto/platform/` SHALL hold a byte-identical copy of the platform-core file after this change, and the contract SHALL pass `buf lint` and `buf breaking` against `origin/main`.

#### Scenario: Copies match the source
- **WHEN** each vendored `proto/platform/{order,payment,promotion}/v1/*.proto` in the 14, 14 and 7 consumer repos is compared with the platform-core file
- **THEN** all 35 copies are byte-identical
- **VERIFIED BY**: `platform-core/packages/proto/monetization_consumer_copies_test.sh` "Copies match the source"

#### Scenario: Drifted or missing copy is rejected
- **WHEN** the check runs on a tree where one vendored copy has an extra byte and another is missing
- **THEN** it exits non-zero and names both files
- **VERIFIED BY**: `platform-core/packages/proto/monetization_consumer_copies_test.sh` "Drifted or missing copy is rejected"

#### Scenario: Renumbering is rejected
- **WHEN** a scratch copy of the proto module renumbers one new field or enum value and `buf breaking` runs against the real module
- **THEN** it fails and names the changed element, for at least `items_subtotal`, `COMMISSION`, `ChargeSellerFeeRequest.amount`, `term_days`, `impression_token` and `PAUSED_BUDGET`
- **VERIFIED BY**: `platform-core/packages/proto/monetization_consumer_copies_test.sh` "Renumbering is rejected"

#### Scenario: Lint and breaking check pass
- **WHEN** `buf lint` and `buf breaking --against '.git#branch=origin/main'` (FILE) run in `platform-core/packages/proto`
- **THEN** both exit 0
- **VERIFIED BY**: `platform-core/packages/proto/monetization_consumer_copies_test.sh` "Lint and breaking check pass"
