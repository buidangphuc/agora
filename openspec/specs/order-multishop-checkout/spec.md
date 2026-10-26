# order-multishop-checkout Specification

## Purpose
Multi-shop checkout quote and order: per-shop groups, prorated platform discount, shop-scoped vouchers.

## Requirements

### Requirement: PreviewCheckout quotes every shop group and reserves nothing

`OrderService.PreviewCheckout` SHALL, for a USER principal, return one `CheckoutGroupQuote` per seller group of the selected cart lines (`item_ids`, empty = whole cart), sorted by `seller_id`, each with `items_subtotal`, `shipping_fee`, `is_free_shipping`, `shop_voucher_code`, `shop_discount_amount`, `platform_discount_amount`, `total_amount` and `estimated_delivery_at`, plus the response `items_subtotal`, `shipping_fee`, `discount_amount`, `total_amount`, `shipping_quoted` and `shipping_address`. It SHALL run the same quote function as `CreateOrder`, reserve no stock, no flash stock and no saga, and create no order. Each group's `shipping_fee` SHALL be `CalculateShippingFee(city, group.items_subtotal)`, so the free-shipping threshold of 500000 applies to a group's own subtotal and never to the selection. `item_ids` matching no cart line SHALL fail `FAILED_PRECONDITION` "cart is empty". When the buyer has no saved address at all the response SHALL carry `shipping_quoted=false`, group `shipping_fee` 0 and no `estimated_delivery_at`; an explicit `address_id` that is not the buyer's, or an empty `address_id` for a buyer who has addresses but none marked default (no first-address fallback), SHALL fail `FAILED_PRECONDITION` "address not found"; identity unreachable (or no identity client) SHALL fail `UNAVAILABLE`, the same fail-closed rule as `CreateOrder`. The only write SHALL be the q5 price pre-check re-pricing the buyer's own changed cart lines ("price changed"). The call SHALL act only on the calling principal's own cart and addresses.

#### Scenario: Multishop checkout: Two shops are quoted per group, not as one total
- **WHEN** a buyer with an address in Đà Nẵng previews a selection of one 300000 line from shop A and one 300000 line from shop B
- **THEN** the response holds two groups, each with items_subtotal 300000, shipping_fee 35000 and is_free_shipping false, and total_amount 670000
- **VERIFIED BY**: platform-e2e `features/order/multishop_checkout.feature` "Multishop checkout: Two shops are quoted per group, not as one total"

#### Scenario: Multishop checkout: Free shipping is decided per group
- **WHEN** a buyer previews a selection of 600000 from shop A and 100000 from shop B
- **THEN** the shop A group has shipping_fee 0 and is_free_shipping true and the shop B group has a non-zero shipping_fee
- **VERIFIED BY**: team-order `internal/service` `TestQuoteCheckoutFreeShippingPerGroup`

#### Scenario: Multishop checkout: Preview totals equal the placed orders
- **WHEN** a buyer previews a two-shop selection and then places the same selection with the same addresses and vouchers
- **THEN** the preview total_amount equals the sum of the two placed orders' total_amount
- **VERIFIED BY**: platform-e2e `features/order/multishop_checkout.feature` "Multishop checkout: Preview totals equal the placed orders"

#### Scenario: Multishop checkout: Preview reserves nothing
- **WHEN** a buyer previews a two-shop selection that carries a shop voucher
- **THEN** no stock reservation, flash hold, saga or order exists afterwards and the cart is unchanged
- **VERIFIED BY**: team-order `internal/service` `TestPreviewCheckoutReservesNothing`

#### Scenario: Multishop checkout: Preview limits itself to the selected lines
- **WHEN** a buyer with three cart lines across two shops previews with item_ids naming one line
- **THEN** the response holds one group containing only that line's item id
- **VERIFIED BY**: team-order `internal/service` `TestPreviewCheckoutSelectsItemIDs`

#### Scenario: Multishop checkout: Preview of an empty selection is refused
- **WHEN** a buyer previews with item_ids that match no cart line
- **THEN** the call fails FAILED_PRECONDITION with the message "cart is empty"
- **VERIFIED BY**: team-order `internal/handler` `TestPreviewCheckoutEmptySelectionFailedPrecondition`

#### Scenario: Multishop checkout: Preview without an address quotes no shipping
- **WHEN** a buyer without any saved address previews a selection of 300000
- **THEN** the response has shipping_quoted false, shipping_fee 0, no estimated_delivery_at and total_amount 300000
- **VERIFIED BY**: team-order `internal/service` `TestPreviewCheckoutNoAddressNotQuoted`

#### Scenario: Multishop checkout: Preview with an unknown address id is refused
- **WHEN** a buyer previews with an address_id that is not one of their addresses
- **THEN** the call fails FAILED_PRECONDITION with the message "address not found"
- **VERIFIED BY**: team-order `internal/handler` `TestPreviewCheckoutUnknownAddressFailedPrecondition`

#### Scenario: Multishop checkout: Preview with addresses but no default is refused
- **WHEN** a buyer who has two saved addresses, none marked default, previews with an empty address_id
- **THEN** the call fails FAILED_PRECONDITION with the message "address not found" and no address is picked as a fallback
- **VERIFIED BY**: team-order `internal/service` `TestPreviewCheckoutNoDefaultAddressFailedPrecondition`

#### Scenario: Multishop checkout: Preview fails closed when identity is unreachable
- **WHEN** a buyer previews a selection while ListAddresses fails with a transport error
- **THEN** the call fails UNAVAILABLE and no voucher hold is left
- **VERIFIED BY**: team-order `internal/service` `TestPreviewCheckoutIdentityUnavailable`

#### Scenario: Multishop checkout: Preview echoes the address it quoted
- **WHEN** a buyer with a default address previews a selection
- **THEN** shipping_address equals that address and each group carries an estimated_delivery_at
- **VERIFIED BY**: team-order `internal/service` `TestPreviewCheckoutEchoesShippingAddress`

#### Scenario: Multishop checkout: Preview refuses a changed price
- **WHEN** a buyer previews a cart line whose cart unit_price differs from its current price
- **THEN** the call fails FAILED_PRECONDITION with the message "price changed" and the line is re-priced to the current price
- **VERIFIED BY**: team-order `internal/service` `TestPreviewCheckoutPriceChangedRepricesCart`

#### Scenario: Multishop checkout: Preview of the buyer's own listing is refused
- **WHEN** a buyer previews a selection that contains a listing of their own shop
- **THEN** the call fails FAILED_PRECONDITION with the message "cannot buy your own listing"
- **VERIFIED BY**: team-order `internal/service` `TestPreviewCheckoutSelfPurchaseRefused`

#### Scenario: Multishop checkout: Preview ignores another buyer's cart
- **WHEN** buyer B previews with item_ids naming a cart line of buyer A
- **THEN** the call fails FAILED_PRECONDITION with the message "cart is empty" and buyer A's cart is unchanged
- **VERIFIED BY**: platform-e2e `features/order/multishop_checkout.feature` "Multishop checkout: Preview ignores another buyer's cart"

#### Scenario: Multishop checkout: Preview needs a user principal
- **WHEN** a service principal, or a call without any principal, invokes PreviewCheckout
- **THEN** the service principal gets PERMISSION_DENIED and the call without a principal gets UNAUTHENTICATED
- **VERIFIED BY**: team-order `internal/handler` `TestPreviewCheckoutRequiresUserPrincipal`

#### Scenario: Multishop checkout: Preview is reachable through the gateway for a signed-in buyer
- **WHEN** a signed-in buyer calls PreviewCheckout through the gateway, and an anonymous visitor makes the same call
- **THEN** the buyer receives the quote and the anonymous call is rejected as unauthenticated
- **VERIFIED BY**: platform-e2e `features/order/multishop_checkout.feature` "Multishop checkout: Preview is reachable through the gateway for a signed-in buyer"

### Requirement: Platform discount is prorated by group subtotal

The quote function SHALL split a platform voucher discount `D` computed once on the selection subtotal across the groups: each group's `platform_discount_amount` SHALL be `D * group.items_subtotal / selection_subtotal` by int64 integer division, and the remainder `D - sum(shares)` SHALL go to the group with the largest `items_subtotal`, ties to the lowest `seller_id`. A group `total_amount` SHALL be `items_subtotal + shipping_fee - shop_discount_amount - platform_discount_amount`, clamped at 0. The shares SHALL sum to `D` and the response `total_amount` SHALL equal the sum of the group totals.

#### Scenario: Multishop checkout: Platform discount splits across groups and sums back
- **WHEN** a platform voucher discount of 10001 is split over groups of items_subtotal 200000, 100000 and 100000
- **THEN** the shares are 5001, 2500 and 2500 and sum to 10001
- **VERIFIED BY**: team-order `internal/service` `TestQuoteCheckoutProratesPlatformDiscount`

#### Scenario: Multishop checkout: Remainder ties go to the lowest seller id
- **WHEN** a platform discount of 100000 is split over three groups of equal items_subtotal 100000
- **THEN** the shares are 33334 for the lowest seller id and 33333 for each of the others
- **VERIFIED BY**: team-order `internal/service` `TestQuoteCheckoutRemainderTieLowestSellerID`

#### Scenario: Multishop checkout: A single group takes the whole platform discount
- **WHEN** a platform discount of 50000 applies to a selection with one group
- **THEN** that group's platform_discount_amount is 50000
- **VERIFIED BY**: team-order `internal/service` `TestQuoteCheckoutSingleGroupWholeDiscount`

#### Scenario: Multishop checkout: Stacked discounts never make a negative total
- **WHEN** a group of items_subtotal 100000 and shipping_fee 35000 carries a shop discount of 100000 and a platform share of 60000
- **THEN** that group's total_amount is 0 and the response total_amount equals the sum of the group totals
- **VERIFIED BY**: team-order `internal/service` `TestQuoteCheckoutClampsGroupTotalAtZero`

#### Scenario: Multishop checkout: Totals always add up
- **WHEN** the quote function runs over a table of selections with 1 to 5 groups, free and paid shipping, and shop and platform discounts
- **THEN** for every row the sum of group totals equals the response total_amount and the platform shares equal D
- **VERIFIED BY**: team-order `internal/service` `TestQuoteCheckoutTableTotalsInvariant`

### Requirement: A shop voucher discounts only its own group

For each entry of `shop_vouchers`, the system SHALL call `ValidateAndReserve` once with that entry's `seller_id` and the group's `items_subtotal` and SHALL apply the returned discount to that group only (`shop_voucher_code`, `shop_discount_amount`). At `CreateOrder` the reservation id SHALL be that group's order id. A shop voucher refused by promotion (including a voucher of another shop), a code whose echoed `scope` is not SHOP or whose echoed `seller_id` differs from the slot's, and a `seller_id` naming no group of the selection SHALL be refused: `PreviewCheckout` SHALL report the voucher with `valid=false` and the reason and quote the totals without it; `CreateOrder` SHALL abort the whole checkout with `FAILED_PRECONDITION` and the reason, place no order and leave no hold. A second `shop_vouchers` entry for one `seller_id` is a malformed request, not a refused voucher: both `PreviewCheckout` and `CreateOrder` SHALL fail `INVALID_ARGUMENT` before any promotion call.

#### Scenario: Multishop checkout: Each shop voucher discounts its own group
- **WHEN** a buyer applies shop A's voucher (10% off) and shop B's voucher (50000 off) to a two-shop selection of 300000 each
- **THEN** group A has shop_discount_amount 30000, group B has 50000, and each placed order stores its own voucher_code and discount
- **VERIFIED BY**: platform-e2e `features/order/multishop_checkout.feature` "Multishop checkout: Each shop voucher discounts its own group"

#### Scenario: Multishop checkout: A shop voucher on another shop's group is refused
- **WHEN** a buyer sends shop A's voucher code in a shop_vouchers entry for shop B's seller_id
- **THEN** the preview reports that voucher valid=false with promotion's reason and the totals carry no shop discount for either group
- **VERIFIED BY**: platform-e2e `features/order/multishop_checkout.feature` "Multishop checkout: A shop voucher on another shop's group is refused"

#### Scenario: Multishop checkout: CreateOrder aborts on a refused shop voucher and holds nothing
- **WHEN** a buyer places a two-shop checkout with shop A's voucher in shop B's slot
- **THEN** the call fails FAILED_PRECONDITION with promotion's reason, no order exists and every stock and voucher hold of the attempt is released
- **VERIFIED BY**: team-order `internal/service` `TestCreateOrderShopVoucherWrongShopAbortsAndReleases`

#### Scenario: Multishop checkout: A platform code in a shop slot is refused
- **WHEN** a buyer sends a platform voucher code in a shop_vouchers entry
- **THEN** the voucher is refused because the echoed scope is not SHOP, and the preview reports valid=false
- **VERIFIED BY**: team-order `internal/service` `TestShopSlotRefusesPlatformScope`

#### Scenario: Multishop checkout: A shop voucher for a shop outside the selection is refused
- **WHEN** a buyer sends a shop_vouchers entry whose seller_id has no group in the selection
- **THEN** the preview reports it valid=false with the reason "voucher does not apply to this checkout" and no promotion call is made for it
- **VERIFIED BY**: team-order `internal/service` `TestShopVoucherSellerNotInSelectionRefused`

#### Scenario: Multishop checkout: Two shop vouchers for one seller are rejected
- **WHEN** a buyer sends two shop_vouchers entries with the same seller_id, once to PreviewCheckout and once to CreateOrder
- **THEN** both calls fail INVALID_ARGUMENT, no promotion call is made and no order is placed
- **VERIFIED BY**: team-order `internal/handler` `TestShopVouchersDuplicateSellerInvalidArgument`

### Requirement: One platform voucher is computed once and split across groups

The system SHALL validate a platform voucher with one `ValidateAndReserve` call carrying an empty `seller_id` and `cart_subtotal` equal to the selection subtotal (sum of every group's `items_subtotal`), under reservation id `platform:<checkout_id>` at `CreateOrder`, so its `min_spend` is checked once on the whole selection and quota is consumed once. The returned discount SHALL be prorated across the groups by the quote function, and each placed order SHALL store `platform_voucher_code` and its own `platform_discount_amount`; `Order.discount_amount` SHALL be `shop discount + platform share`. A code whose echoed `scope` is not PLATFORM in `platform_voucher_code` SHALL be refused. A refused platform voucher SHALL be reported `valid=false` with the reason in `PreviewCheckout` (totals quoted without it) and SHALL abort `CreateOrder` with `FAILED_PRECONDITION` and the reason, with no hold left.

#### Scenario: Multishop checkout: Platform voucher is split and stored per order
- **WHEN** a buyer places a two-shop checkout of 300000 per shop with a platform voucher worth 60000
- **THEN** each order has platform_discount_amount 30000, the same platform_voucher_code, the same checkout_id, and total_amount 305000
- **VERIFIED BY**: platform-e2e `features/order/multishop_checkout.feature` "Multishop checkout: Platform voucher is split and stored per order"

#### Scenario: Multishop checkout: Platform min spend is checked on the whole selection
- **WHEN** a buyer applies a platform voucher with min_spend 500000 to two groups of 300000
- **THEN** the voucher is valid with one ValidateAndReserve call whose cart_subtotal is 600000
- **VERIFIED BY**: team-order `internal/service` `TestPlatformVoucherMinSpendOnSelectionSubtotal`

#### Scenario: Multishop checkout: Platform voucher below min spend is refused
- **WHEN** a buyer applies a platform voucher with min_spend 700000 to a selection of 600000
- **THEN** the preview reports valid=false with promotion's reason and the totals carry no platform discount
- **VERIFIED BY**: platform-e2e `features/order/multishop_checkout.feature` "Multishop checkout: Platform voucher below min spend is refused"

#### Scenario: Multishop checkout: A shop code in the platform slot is refused
- **WHEN** a buyer sends a shop voucher code as platform_voucher_code
- **THEN** the voucher is refused because the echoed scope is not PLATFORM, and CreateOrder fails FAILED_PRECONDITION
- **VERIFIED BY**: team-order `internal/service` `TestPlatformSlotRefusesShopScope`

#### Scenario: Multishop checkout: Platform and shop vouchers stack on one group
- **WHEN** a buyer applies shop A's 30000 voucher and a platform voucher worth 60000 to a two-shop selection of 300000 each
- **THEN** group A has shop_discount_amount 30000 and platform_discount_amount 30000, and group A total_amount is 275000 (300000 + 35000 - 60000)
- **VERIFIED BY**: team-order `internal/service` `TestQuoteCheckoutStacksShopAndPlatform`

#### Scenario: Multishop checkout: CreateOrder aborts on a refused platform voucher and holds nothing
- **WHEN** a buyer places a two-shop checkout with a platform voucher that promotion refuses
- **THEN** the call fails FAILED_PRECONDITION with promotion's reason, no order exists and the stock holds and any shop voucher hold of the attempt are released
- **VERIFIED BY**: team-order `internal/service` `TestCreateOrderPlatformVoucherRefusedAbortsAndReleases`

#### Scenario: Multishop checkout: Every voucher hold of a failed checkout is released
- **WHEN** a checkout holding two shop vouchers and a platform voucher fails in a later phase
- **THEN** failAndCompensate releases both shop holds under their order ids and the platform hold under platform:<checkout_id>
- **VERIFIED BY**: team-order `internal/service` `TestFailAndCompensateReleasesAllVoucherHolds`

### Requirement: The legacy voucher_code is the platform-voucher slot only

When `voucher_code` is set, the system SHALL treat it exactly as `platform_voucher_code` (one `ValidateAndReserve` with an empty `seller_id`, the selection subtotal and reservation id `platform:<checkout_id>`), with no SHOP auto-routing and no probe. A code whose echoed `scope` is not PLATFORM (a SHOP voucher, whatever seller it names, inside or outside the selection) and a code promotion does not know (it answers `valid=false`) SHALL be refused like any platform-slot refusal: `PreviewCheckout` SHALL report it `valid=false` with the reason and quote the totals without it; `CreateOrder` SHALL fail `FAILED_PRECONDITION` with the reason, place no order and leave no hold. `voucher_code` set together with `platform_voucher_code`, or repeating a `shop_vouchers` code, is a malformed request: both RPCs SHALL fail `INVALID_ARGUMENT` before any promotion call.

#### Scenario: Multishop checkout: A legacy platform code becomes the platform voucher
- **WHEN** a client sends only voucher_code with a platform voucher on a two-shop selection
- **THEN** the discount is split across both groups as a platform voucher
- **VERIFIED BY**: team-order `internal/service` `TestLegacyVoucherCodePlatformScopeRouted`

#### Scenario: Multishop checkout: A legacy shop code is refused as a platform voucher
- **WHEN** a client sends only voucher_code with shop B's voucher on a two-shop selection, once to PreviewCheckout and once to CreateOrder, and again with the voucher of a shop that has no group in the selection
- **THEN** the preview reports the code valid=false because the echoed scope is not PLATFORM with no shop discount on any group, CreateOrder fails FAILED_PRECONDITION with no order placed and no hold left, and no second ValidateAndReserve is made for a seller group
- **VERIFIED BY**: team-order `internal/service` `TestLegacyVoucherCodeShopScopeRefused`

#### Scenario: Multishop checkout: An unknown legacy code is refused
- **WHEN** a client sends voucher_code with a code promotion does not know, once to PreviewCheckout and once to CreateOrder
- **THEN** the preview reports it valid=false with promotion's reason and the totals carry no discount, and CreateOrder fails FAILED_PRECONDITION with that reason, places no order and leaves no hold
- **VERIFIED BY**: team-order `internal/service` `TestLegacyVoucherCodeUnknownRefused`

#### Scenario: Multishop checkout: A voucher named twice is rejected
- **WHEN** a client sends the same code in voucher_code and in shop_vouchers, to PreviewCheckout and to CreateOrder
- **THEN** both calls fail INVALID_ARGUMENT and no promotion call is made
- **VERIFIED BY**: team-order `internal/handler` `TestVoucherCodeNamedTwiceInvalidArgument`

#### Scenario: Multishop checkout: Legacy and platform voucher fields together are rejected
- **WHEN** a client sets both voucher_code and platform_voucher_code, to PreviewCheckout and to CreateOrder
- **THEN** both calls fail INVALID_ARGUMENT and no promotion call is made
- **VERIFIED BY**: team-order `internal/handler` `TestLegacyAndPlatformVoucherTogetherInvalidArgument`

#### Scenario: Multishop checkout: Vouchers are ignored without a promotion client
- **WHEN** CreateOrder runs in a team-order started without `UPSTREAM_PROMOTION_ADDR` with vouchers in the request, and PreviewCheckout is called the same way
- **THEN** the order is placed with no discount and the preview reports every voucher valid=false with the reason "vouchers unavailable"
- **VERIFIED BY**: team-order `internal/service` `TestVouchersIgnoredWithoutPromotionClient`

#### Scenario: Multishop checkout: A promotion outage blocks a voucher checkout
- **WHEN** `ValidateAndReserve` fails with a transport error during CreateOrder or PreviewCheckout
- **THEN** the call fails UNAVAILABLE, no order is placed and every hold of the attempt is released
- **VERIFIED BY**: team-order `internal/service` `TestVoucherTransportErrorUnavailable`

### Requirement: Preview releases its voucher holds and never commits

`PreviewCheckout` SHALL validate each applied voucher under a reservation id `preview:<buyer id>:<random>` as team-order's service principal and SHALL call `ReleaseReservation` for each right after quoting, best effort. It SHALL never call `CommitReservation`. A release failure SHALL be logged and SHALL NOT fail the preview.

#### Scenario: Multishop checkout: Preview holds are released and never committed
- **WHEN** a buyer previews a selection with one shop voucher and one platform voucher
- **THEN** two ValidateAndReserve calls carry preview-namespace ids, two ReleaseReservation calls follow and no CommitReservation call is made
- **VERIFIED BY**: team-order `internal/service` `TestPreviewCheckoutReleasesPreviewHoldsNeverCommits`

#### Scenario: Multishop checkout: A preview release failure does not fail the preview
- **WHEN** `ReleaseReservation` fails during a preview
- **THEN** the preview still returns its quote
- **VERIFIED BY**: team-order `internal/service` `TestPreviewCheckoutReleaseErrorIgnored`

### Requirement: CreateOrder places each order at the quoted figures

`CreateOrder` SHALL place one order per seller group at exactly the figures `quoteCheckout` yields for the same inputs: `items_subtotal`, `shipping_fee`, `discount_amount` (shop discount + platform share), `total_amount`, the shop `voucher_code`, `platform_voucher_code` and `platform_discount_amount`. Every order of one checkout SHALL carry the same `checkout_id` equal to the saga id. `CreateOrder` SHALL read `shop_vouchers`, `platform_voucher_code` and `voucher_code` and keep placing orders of a no-voucher checkout as today. Shipping SHALL never be quoted for an empty city (address integrity is `order-address-eta`).

#### Scenario: Multishop checkout: Orders of one checkout share a checkout_id
- **WHEN** a buyer places a three-shop checkout
- **THEN** the three returned orders carry the same non-empty checkout_id and GetOrder returns it for each
- **VERIFIED BY**: platform-e2e `features/order/multishop_checkout.feature` "Multishop checkout: Orders of one checkout share a checkout_id"

#### Scenario: Multishop checkout: Placed totals equal the quote function
- **WHEN** runCheckout places a two-shop checkout with a shop voucher and a platform voucher
- **THEN** each stored order equals its group row of quoteCheckout in shipping_fee, discount_amount and total_amount
- **VERIFIED BY**: team-order `internal/service` `TestRunCheckoutPlacesQuotedFigures`

#### Scenario: Multishop checkout: A checkout without vouchers is unchanged
- **WHEN** a buyer places a two-shop checkout with no voucher field set
- **THEN** each order has discount_amount 0, empty platform_voucher_code and total_amount equal to items_subtotal plus its shipping_fee
- **VERIFIED BY**: team-order `internal/service` `TestRunCheckoutNoVoucherUnchanged`

#### Scenario: Multishop checkout: A non-user principal cannot place a multi-voucher checkout
- **WHEN** a service principal calls CreateOrder with shop_vouchers and a platform_voucher_code
- **THEN** the call fails PERMISSION_DENIED and no promotion call is made
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderVouchersRequireUserPrincipal`

#### Scenario: Multishop checkout: The order repository returns checkout siblings
- **WHEN** ListOrdersByCheckout is called with a checkout_id of three placed orders, and with an unknown checkout_id
- **THEN** the first call returns the three orders and the second returns none
- **VERIFIED BY**: team-order `internal/repository` `TestListOrdersByCheckout`

#### Scenario: Multishop checkout: Order rows stay readable before the migration data exists
- **WHEN** an order placed before the new columns existed is read
- **THEN** its checkout_id and platform_voucher_code are empty and its platform_discount_amount is 0
- **VERIFIED BY**: team-order `internal/repository` `TestOrderLegacyRowDefaultsNewColumns`

### Requirement: The platform voucher is committed on the first paid order

When an order with a `platform_voucher_code` settles as paid, the payment consumer SHALL call `CommitReservation("platform:<checkout_id>")`; a repeat for a later paid order of the same checkout SHALL be an idempotent no-op in promotion, so the voucher counts as used once. A commit failure SHALL fail the event for retry as the shop voucher commit does. An order without a `platform_voucher_code` SHALL cause no platform commit.

#### Scenario: Multishop checkout: First paid order commits the platform voucher
- **WHEN** the first order of a two-order checkout with a platform voucher settles as paid
- **THEN** CommitReservation is called once with platform:<checkout_id>
- **VERIFIED BY**: team-order `internal/consumer` `TestSettleCommitsPlatformVoucherOnFirstPaid`

#### Scenario: Multishop checkout: Platform voucher is counted once for two paid orders
- **WHEN** the second order of the same checkout settles after the first
- **THEN** CommitReservation is called again with the same id and the voucher's used count rises once in total
- **VERIFIED BY**: platform-e2e `features/order/multishop_checkout.feature` "Multishop checkout: Platform voucher is counted once for two paid orders"

#### Scenario: Multishop checkout: An order without a platform voucher makes no platform commit
- **WHEN** an order with only a shop voucher settles as paid
- **THEN** only the shop voucher is committed under the order id
- **VERIFIED BY**: team-order `internal/consumer` `TestSettleNoPlatformCommitWithoutPlatformVoucher`

#### Scenario: Multishop checkout: A platform commit failure is retried
- **WHEN** CommitReservation for the platform voucher fails with a transport error during settle
- **THEN** the consumer returns the error so the event is retried, not acknowledged
- **VERIFIED BY**: team-order `internal/consumer` `TestSettlePlatformCommitErrorRetried`

### Requirement: The platform voucher is released only when the whole checkout is cancelled unpaid

After a cancel claim, on every cancel path (buyer `CancelOrder` and the system cancel of the unpaid-order expiry sweeper, through the same helper keyed by `checkout_id`), the system SHALL release the platform hold `platform:<checkout_id>` only when every order of the checkout is Cancelled and none has `paid_at`; otherwise it SHALL release nothing and SHALL NOT re-price any sibling. A release failure SHALL be logged and SHALL NOT fail the cancel. Shop voucher release by order id is unchanged.

#### Scenario: Multishop checkout: Cancelling one order keeps the platform voucher and sibling prices
- **WHEN** a buyer cancels one of two unpaid orders of a checkout that used a platform voucher
- **THEN** no platform release call is made and the other order's total_amount and platform_discount_amount are unchanged
- **VERIFIED BY**: platform-e2e `features/order/multishop_checkout.feature` "Multishop checkout: Cancelling one order keeps the platform voucher and sibling prices"

#### Scenario: Multishop checkout: Cancelling the last unpaid order releases the platform voucher
- **WHEN** the second and last unpaid order of the checkout is cancelled after the first
- **THEN** ReleaseReservation is called once with platform:<checkout_id>
- **VERIFIED BY**: team-order `internal/service` `TestCancelLastOrderReleasesPlatformVoucher`

#### Scenario: Multishop checkout: A paid sibling blocks the platform release
- **WHEN** one order of the checkout is paid and the other is cancelled
- **THEN** no platform release call is made
- **VERIFIED BY**: team-order `internal/service` `TestCancelWithPaidSiblingKeepsPlatformVoucher`

#### Scenario: Multishop checkout: The system cancel of an expired order follows the same rule
- **WHEN** the unpaid-order expiry sweeper cancels the last unpaid order of a checkout whose siblings are all cancelled
- **THEN** ReleaseReservation is called once with platform:<checkout_id>
- **VERIFIED BY**: team-order `internal/service` `TestSystemCancelReleasesPlatformVoucherWhenAllCancelled`

#### Scenario: Multishop checkout: A platform release failure never fails the cancel
- **WHEN** ReleaseReservation for the platform voucher fails while the last order is cancelled
- **THEN** the cancel succeeds with the order Cancelled
- **VERIFIED BY**: team-order `internal/service` `TestCancelPlatformReleaseErrorNeverFailsCancel`

#### Scenario: Multishop checkout: Cancelling an order without a platform voucher makes no platform call
- **WHEN** an order without a platform_voucher_code or checkout_id is cancelled
- **THEN** no ListOrdersByCheckout and no platform release call is made
- **VERIFIED BY**: team-order `internal/service` `TestCancelWithoutPlatformVoucherNoPlatformCalls`

### Requirement: The gateway forwards PreviewCheckout for a signed-in buyer

`team-gateway` SHALL expose `OrderService.PreviewCheckout` through `OrderForwarder`, forwarding the request with the caller's principal and mapping upstream errors with `toConnectErr`, like `CalculateShippingFee`.

#### Scenario: Multishop checkout: The gateway forwards PreviewCheckout with the caller's identity
- **WHEN** a request reaches `OrderForwarder.PreviewCheckout`
- **THEN** the upstream client receives the same request message and the response is returned unchanged
- **VERIFIED BY**: team-gateway `internal/edge` `TestOrderForwarderPreviewCheckout`

#### Scenario: Multishop checkout: The gateway maps an upstream failure
- **WHEN** the upstream PreviewCheckout returns FAILED_PRECONDITION "cart is empty"
- **THEN** the gateway returns the equivalent Connect failed-precondition error with the same message
- **VERIFIED BY**: team-gateway `internal/edge` `TestOrderForwarderPreviewCheckoutMapsError`
