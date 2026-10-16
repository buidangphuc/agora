## ADDED Requirements

### Requirement: AddToCart prices a line at the active flash price

`CartService.AddToCart` SHALL resolve the unit price of the added line with one `FlashSaleService.GetActiveFlashSale(listing_id)` call. The unit price SHALL be the campaign `sale_price` when the campaign is active, its `variant_id` is empty or equals the line's variant_id, and `0 < sale_price < regular price` (the listing price, or the variant price when the variant price is above 0). In every other case the unit price SHALL be the regular price. When the call fails, or no promotion client is configured, the unit price SHALL be the regular price. `Reorder` SHALL price each re-added item through the same path.

#### Scenario: Advertised price: Listing-wide flash price is added to the cart
- **WHEN** a buyer adds quantity 2 of a listing priced 500000 with an active campaign of empty variant_id and sale_price 350000
- **THEN** the cart line unit_price is 350000 and the cart subtotal is 700000
- **VERIFIED BY**: team-order `internal/service` `TestAddToCartFlashPriceListingWide`

#### Scenario: Advertised price: Variant campaign prices only its variant
- **WHEN** a buyer adds variant "v1" and variant "v2" of one listing, and the active campaign names variant "v1" with sale_price 300000 (regular 400000)
- **THEN** the "v1" line unit_price is 300000 and the "v2" line unit_price is 400000
- **VERIFIED BY**: team-order `internal/service` `TestAddToCartFlashPriceVariantOnly`

#### Scenario: Advertised price: Sale price at or above the regular price is ignored
- **WHEN** a buyer adds a listing priced 200000 whose active campaign has sale_price 200000, and another whose campaign has sale_price 250000
- **THEN** both cart lines have the regular unit_price
- **VERIFIED BY**: team-order `internal/service` `TestAddToCartFlashPriceNotBelowRegularIgnored`

#### Scenario: Advertised price: No active campaign keeps the regular price
- **WHEN** a buyer adds a listing for which `GetActiveFlashSale` returns active=false
- **THEN** the cart line unit_price equals the regular price
- **VERIFIED BY**: team-order `internal/service` `TestAddToCartNoCampaignRegularPrice`

#### Scenario: Advertised price: Promotion error at add-to-cart falls back to the regular price
- **WHEN** a buyer adds a listing while `GetActiveFlashSale` returns an error
- **THEN** the call succeeds and the cart line unit_price equals the regular price
- **VERIFIED BY**: team-order `internal/service` `TestAddToCartPromotionErrorRegularPrice`

#### Scenario: Advertised price: Missing promotion client keeps today's behaviour
- **WHEN** a buyer adds a listing in a team-order started without `UPSTREAM_PROMOTION_ADDR`
- **THEN** the call succeeds with the regular price and no promotion call is made
- **VERIFIED BY**: team-order `internal/service` `TestAddToCartNilFlashClientRegularPrice`

#### Scenario: Advertised price: Reorder re-prices through add-to-cart
- **WHEN** a buyer reorders a past order whose item was bought at the regular price while a flash campaign with sale_price 350000 is now active
- **THEN** the cart line unit_price is 350000
- **VERIFIED BY**: team-order `internal/service` `TestReorderUsesFlashPrice`

#### Scenario: Advertised price: Flash price is visible end to end after add-to-cart
- **WHEN** a buyer adds a listing with an active campaign (sale_price below the regular price) through the gateway
- **THEN** `GetCart` returns the line unit_price equal to the campaign sale_price
- **VERIFIED BY**: platform-e2e `features/order/advertised_price.feature` "Flash price is the cart price"

### Requirement: Checkout re-resolves every line and refuses a price change

Before creating the saga or reserving any stock, voucher or flash hold, `CreateOrdersFromCart` SHALL load each selected line's listing from team-domain and resolve its current price with the same rule as AddToCart (one `GetActiveFlashSale` per distinct listing). When any selected line's cart unit_price differs from its current price (higher or lower), the call SHALL fail with `FAILED_PRECONDITION` and the message "price changed", SHALL update each differing cart line's unit_price to its current price, and SHALL hold nothing. A line SHALL be priced entirely at the flash price or entirely at the regular price; it is never split. A line whose cart price equals its current price SHALL proceed unchanged. With an idempotency key, the key SHALL be freed so the retry runs.

#### Scenario: Advertised price: Ended sale refuses checkout and re-prices the cart
- **WHEN** a buyer checks out a cart line priced 350000 after its campaign ended (regular price 500000)
- **THEN** CreateOrder returns `FAILED_PRECONDITION` "price changed", the cart line unit_price is 500000, and no reservation, voucher hold or flash hold exists for the attempt
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutEndedSaleRefusedAndRepriced`

#### Scenario: Advertised price: Sale started after add-to-cart refuses checkout
- **WHEN** a buyer checks out a cart line priced 500000 after a campaign with sale_price 350000 became active for it
- **THEN** CreateOrder returns `FAILED_PRECONDITION` "price changed" and the cart line unit_price is 350000
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutSaleStartedRefusedAndRepriced`

#### Scenario: Advertised price: Retry after the re-price places the order at the new price
- **WHEN** a buyer calls CreateOrder again after a "price changed" refusal without changing the cart
- **THEN** the order is created and its item unit_price equals the re-priced cart unit_price
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutRetryAfterRepriceSucceeds`

#### Scenario: Advertised price: Matching flash price proceeds at the flash price
- **WHEN** a buyer checks out a line priced 350000 whose campaign is still active with sale_price 350000 and enough remaining stock
- **THEN** the order item unit_price is 350000, items_subtotal is 350000 times the quantity, and total_amount equals items_subtotal plus shipping_fee minus discount_amount
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutChargesFlashPrice`

#### Scenario: Advertised price: One changed line refuses the whole checkout
- **WHEN** a buyer checks out two lines of which only the second changed price
- **THEN** CreateOrder returns `FAILED_PRECONDITION` "price changed", the second cart line is re-priced, the first line is unchanged, and no order exists
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutOneChangedLineRefusesAll`

#### Scenario: Advertised price: Refused checkout frees the idempotency key
- **WHEN** a keyed checkout is refused with "price changed" and the buyer retries with the same key after the re-price
- **THEN** the retry creates the order
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutPriceChangedFreesIdempotencyKey`

#### Scenario: Advertised price: Another buyer's cart is never re-priced
- **WHEN** a refused checkout re-prices the cart of buyer "b1" while buyer "b2" holds the same listing in the cart
- **THEN** the "b2" cart line unit_price is unchanged
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutRepriceScopedToBuyer`

#### Scenario: Advertised price: Checkout maps price changed to FAILED_PRECONDITION
- **WHEN** the service returns `ErrPriceChanged`
- **THEN** the CreateOrder handler returns code `FailedPrecondition` with message exactly "price changed"
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderPriceChangedFailedPrecondition`

#### Scenario: Advertised price: Ended sale is refused end to end and the retry is charged the regular price
- **WHEN** a buyer carts a flash-priced listing, the campaign is switched off, and the buyer calls CreateOrder, then GetCart, then CreateOrder again
- **THEN** the first call fails with `failed_precondition` "price changed", the cart shows the regular price, and the second call returns an order whose item unit_price is the regular price
- **VERIFIED BY**: platform-e2e `features/order/advertised_price.feature` "An ended sale is refused and re-priced"

### Requirement: Checkout fails closed when the flash price cannot be confirmed

When `GetActiveFlashSale` returns an error during the checkout pre-check, the line SHALL be decided without the campaign. A line whose cart unit_price is above the regular price SHALL count as a price change (refusal "price changed", re-priced to the regular price). A line whose cart unit_price equals the regular price SHALL proceed at the regular price. A line whose cart unit_price is below the regular price SHALL fail the whole checkout with `UNAVAILABLE`, hold nothing and leave the cart unchanged. When a `ReserveFlashSaleStock` call fails with a transport error, the checkout SHALL fail with `UNAVAILABLE` and release every hold of the attempt. When no promotion client is configured, every line SHALL be priced at the regular price, so a flash-priced cart line is refused as a price change.

#### Scenario: Advertised price: Promotion outage blocks a flash-priced line
- **WHEN** a buyer checks out a line whose cart price is below the regular price while `GetActiveFlashSale` returns an error
- **THEN** CreateOrder returns `UNAVAILABLE`, no reservation exists and the cart line unit_price is unchanged
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutPromotionDownFlashLineUnavailable`

#### Scenario: Advertised price: Promotion outage does not block a regular-priced line
- **WHEN** a buyer checks out a line priced at its regular price while `GetActiveFlashSale` returns an error
- **THEN** the order is created at the regular price
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutPromotionDownRegularLineProceeds`

#### Scenario: Advertised price: Cart price above regular is a price change even when promotion is down
- **WHEN** a buyer checks out a line priced above the listing's current regular price while `GetActiveFlashSale` returns an error
- **THEN** CreateOrder returns `FAILED_PRECONDITION` "price changed" and the cart line unit_price equals the regular price
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutPromotionDownCartAboveRegularPriceChanged`

#### Scenario: Advertised price: Hold transport error releases the attempt
- **WHEN** a checkout of two flash-priced lines gets a transport error on the second `ReserveFlashSaleStock`
- **THEN** CreateOrder returns `UNAVAILABLE`, the first flash hold is released, the domain reservations are released and no order exists
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutFlashHoldTransportErrorUnavailable`

#### Scenario: Advertised price: Unconfigured promotion refuses a flash-priced cart line
- **WHEN** a buyer checks out a line priced below its regular price in a team-order without a promotion client
- **THEN** CreateOrder returns `FAILED_PRECONDITION` "price changed" and the cart line unit_price equals the regular price
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutNilPromotionFlashLineRepriced`

#### Scenario: Advertised price: Checkout maps promotion unavailable to UNAVAILABLE
- **WHEN** the service returns `ErrPromotionUnavailable`
- **THEN** the CreateOrder handler returns code `Unavailable`
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderPromotionUnavailableUnavailable`

### Requirement: Flash-priced lines hold campaign stock in Phase A

For each flash-priced line, `reserveGroup` SHALL call `ReserveFlashSaleStock` after the domain stock reservation, with `reservation_id` = `<order id>:<listing id>:<variant id>` (the seller group's pre-generated order id; empty variant segment for a listing line), the campaign id, listing id, variant id, `quantity` = the line quantity and `expected_sale_price` = the charged unit price. One call SHALL cover the whole line quantity. A response with `reserved=false` SHALL fail the checkout with `FAILED_PRECONDITION` "price changed", release every hold of the attempt (domain stock, voucher, flash holds), re-price the refused line to the regular price and re-price any other differing line to its current price. Regular-priced lines SHALL make no hold call. The voucher `ValidateAndReserve` SHALL receive the flash-priced items subtotal as `cart_subtotal`.

#### Scenario: Advertised price: Flash line takes one hold for its full quantity
- **WHEN** a buyer checks out quantity 3 of a flash-priced line
- **THEN** exactly one `ReserveFlashSaleStock` call is made with quantity 3, `expected_sale_price` equal to the sale price and the reservation id `<order id>:<listing id>:<variant id>`
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutFlashHoldOnePerLineFullQuantity`

#### Scenario: Advertised price: Regular line makes no hold call
- **WHEN** a buyer checks out a line priced at its regular price with no active campaign
- **THEN** no `ReserveFlashSaleStock` call is made
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutRegularLineNoFlashHold`

#### Scenario: Advertised price: Refused hold is a price change and releases everything
- **WHEN** a checkout with a voucher and two flash-priced lines is refused (`reserved=false`) on the second hold
- **THEN** CreateOrder returns `FAILED_PRECONDITION` "price changed", the first flash hold, the voucher hold and the domain reservations are released, the refused line's cart unit_price equals its regular price and no order exists
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutRefusedFlashHoldPriceChangedReleasesAll`

#### Scenario: Advertised price: Voucher is computed on the flash-priced subtotal
- **WHEN** a buyer checks out quantity 2 of a line at flash price 350000 with a voucher code
- **THEN** `ValidateAndReserve` receives `cart_subtotal` 700000
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutVoucherReservedOnFlashSubtotal`

#### Scenario: Advertised price: Hold call never carries the buyer principal
- **WHEN** a buyer's checkout places a flash hold
- **THEN** the outgoing `ReserveFlashSaleStock` call carries scope `promotion.reserve` as principal `service-team-order` and not the buyer's principal id
- **VERIFIED BY**: team-order `internal/upstream` `TestServiceFlashClientSendsServicePrincipal`

#### Scenario: Advertised price: Concurrent buyers cannot exceed the campaign cap end to end
- **WHEN** two buyers each check out quantity 3 of a listing whose campaign has stock_cap 4 and both calls overlap
- **THEN** exactly one order is created at the flash price and the other call fails with `failed_precondition` "price changed"
- **VERIFIED BY**: platform-e2e `features/order/advertised_price.feature` "Concurrent buyers cannot exceed the flash cap"

### Requirement: Flash holds are committed in Phase B

Before `commitDomainReservations` runs, `runCheckout` SHALL call `CommitFlashSaleStock(reservation_id)` for every flash hold of the attempt, so a flash commit failure leaves the domain reservations uncommitted. A commit error or `committed=false` SHALL fail the attempt through the existing compensation path: `failAndCompensate` runs and CreateOrder returns `UNAVAILABLE`. A domain commit failure after the flash commits SHALL release those flash holds (committed units are returned) through `failAndCompensate`. The consumer on OrderPaid SHALL NOT call any flash RPC. Every checkout attempt SHALL generate fresh order ids, so a reservation id is never reused after its hold was released.

#### Scenario: Advertised price: Flash hold is committed before the order exists
- **WHEN** a checkout with one flash-priced line reaches Phase B
- **THEN** `CommitFlashSaleStock` is called once with the line's reservation id before `commitDomainReservations` and before `PlaceOrders` run
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutCommitsFlashHoldInPhaseB`

#### Scenario: Advertised price: Flash commit failure compensates with domain stock uncommitted
- **WHEN** `CommitFlashSaleStock` returns an error in Phase B
- **THEN** CreateOrder returns `UNAVAILABLE`, no domain commit call was made, every flash hold and domain reservation is released, the saga is COMPENSATED and no order exists
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutFlashCommitErrorCompensates`

#### Scenario: Advertised price: Domain commit failure releases committed flash holds
- **WHEN** the flash commit succeeds and `commitDomainReservations` then fails
- **THEN** CreateOrder fails, `ReleaseFlashSaleStock` is called for the committed hold, the saga is COMPENSATED and no order exists
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutDomainCommitFailureReleasesCommittedFlashHold`

#### Scenario: Advertised price: Uncommitted result is refused
- **WHEN** `CommitFlashSaleStock` returns `committed=false` in Phase B
- **THEN** CreateOrder returns `UNAVAILABLE` and no order exists
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutFlashCommitFalseCompensates`

#### Scenario: Advertised price: Regular-only checkout makes no commit call
- **WHEN** a buyer checks out lines that are all regular-priced
- **THEN** no `CommitFlashSaleStock` call is made
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutRegularLineNoFlashCommit`

#### Scenario: Advertised price: Payment settlement makes no flash call
- **WHEN** a PaymentSettled(PAID) event moves an order with a flash-priced item to Paid
- **THEN** no flash RPC is called by the consumer
- **VERIFIED BY**: team-order `internal/consumer` `TestPaymentConsumerMakesNoFlashCall`

#### Scenario: Advertised price: A retry uses fresh reservation ids
- **WHEN** a buyer retries a checkout after a refused one
- **THEN** the retry's flash `reservation_id` differs from the refused attempt's
- **VERIFIED BY**: team-order `internal/service` `TestCheckoutRetryUsesFreshFlashReservationIDs`

#### Scenario: Advertised price: Placing a flash-priced order grows stock_sold end to end
- **WHEN** a buyer places a flash-priced order of quantity 2
- **THEN** the campaign's `stock_sold` is 2 higher without waiting for payment
- **VERIFIED BY**: platform-e2e `features/order/advertised_price.feature` "A placed flash order counts as sold"

### Requirement: Flash holds are released on compensation and cancel

`failAndCompensate` SHALL release every flash hold recorded for the failed attempt (flash-priced lines only, committed or not) through `ReleaseFlashSaleStock`, on a background context, best effort and idempotent. `CancelOrder` SHALL, after winning the cancel claim, call `ReleaseFlashSaleStock` for the hold id of every item of the cancelled order, because the order item stores no price source; an id that never had a hold is a no-op in promotion, and a committed hold's units are returned. A release error SHALL be logged and SHALL NOT fail the cancel or change the checkout error. With no promotion client, no release call SHALL be made.

#### Scenario: Advertised price: Failed checkout returns its flash holds
- **WHEN** Phase B (domain commit) fails after a flash hold was placed
- **THEN** `ReleaseFlashSaleStock` is called once with that reservation id and the saga is COMPENSATED
- **VERIFIED BY**: team-order `internal/service` `TestFailAndCompensateReleasesFlashHolds`

#### Scenario: Advertised price: Compensation releases flash lines only
- **WHEN** a checkout with one flash-priced and one regular line fails in Phase B
- **THEN** `ReleaseFlashSaleStock` is called once, for the flash line's reservation id
- **VERIFIED BY**: team-order `internal/service` `TestFailAndCompensateReleasesOnlyFlashLines`

#### Scenario: Advertised price: Cancel returns the flash hold
- **WHEN** a buyer cancels a Pending order that has a flash-priced item
- **THEN** `ReleaseFlashSaleStock` is called with the item's hold id and the order is Cancelled
- **VERIFIED BY**: team-order `internal/service` `TestCancelOrderReleasesFlashHold`

#### Scenario: Advertised price: Cancel calls release for every item
- **WHEN** a buyer cancels an order with one flash-priced and one regular item
- **THEN** `ReleaseFlashSaleStock` is called once per item and the order is Cancelled
- **VERIFIED BY**: team-order `internal/service` `TestCancelOrderReleasesEveryItem`

#### Scenario: Advertised price: Flash release failure never fails the cancel
- **WHEN** `ReleaseFlashSaleStock` returns an error during CancelOrder
- **THEN** the cancel succeeds, the order is Cancelled and the error is logged
- **VERIFIED BY**: team-order `internal/service` `TestCancelOrderFlashReleaseErrorStillCancels`

#### Scenario: Advertised price: Cancel of an order that is not Pending or Paid releases nothing
- **WHEN** a buyer cancels a Shipped order
- **THEN** the call is refused with an invalid-status error and no `ReleaseFlashSaleStock` call is made
- **VERIFIED BY**: team-order `internal/service` `TestCancelOrderRefusedNoFlashRelease`

#### Scenario: Advertised price: Cancel of a Paid order releases the committed hold
- **WHEN** a buyer cancels a Paid order that has a flash-priced item
- **THEN** `ReleaseFlashSaleStock` is called with the item's hold id and the order is Cancelled
- **VERIFIED BY**: team-order `internal/service` `TestCancelPaidOrderReleasesCommittedFlashHold`

#### Scenario: Advertised price: Cancelled placed order returns flash units end to end
- **WHEN** a buyer places a flash-priced order for quantity 2 of a campaign with stock_cap 2 (the hold is already committed) and cancels it
- **THEN** `stock_sold` decreases by 2, `GetFlashSaleStock.remaining` is 2 again and a second buyer's flash-priced checkout of quantity 2 succeeds
- **VERIFIED BY**: platform-e2e `features/order/advertised_price.feature` "A cancelled order returns its flash units"

### Requirement: Flash RPCs are sent as team-order's service principal

`upstream.DialPromotion` SHALL return a client set whose `Flash` client sends `ReserveFlashSaleStock`, `CommitFlashSaleStock` and `ReleaseFlashSaleStock` with `x-principal-id=service-team-order`, `x-principal-type=service` and scope `promotion.reserve`, replacing any forwarded caller principal, and SHALL leave `GetActiveFlashSale` with the forwarded principal. An empty address SHALL return a nil client set.

#### Scenario: Advertised price: Hold RPCs carry the service principal
- **WHEN** the wrapped flash client sends a reserve, a commit and a release inside a buyer's request context
- **THEN** each outgoing call carries principal id `service-team-order`, type `service` and scope `promotion.reserve`, and not the buyer's id
- **VERIFIED BY**: team-order `internal/upstream` `TestServiceFlashClientSendsServicePrincipal`

#### Scenario: Advertised price: Price read keeps the forwarded principal
- **WHEN** the wrapped flash client sends `GetActiveFlashSale` inside a buyer's request context
- **THEN** the outgoing call carries the buyer's principal unchanged
- **VERIFIED BY**: team-order `internal/upstream` `TestServiceFlashClientReadNotWrapped`

#### Scenario: Advertised price: Unset promotion address yields no client
- **WHEN** `DialPromotion` is called with an empty address
- **THEN** it returns a nil client set and a nil error
- **VERIFIED BY**: team-order `internal/upstream` `TestDialPromotionEmptyAddrNil`

#### Scenario: Advertised price: A buyer token cannot reserve flash stock end to end
- **WHEN** a buyer token calls `ReserveFlashSaleStock` directly through the gateway
- **THEN** the call is refused with `permission_denied`, `not_found` or `unimplemented` and `GetFlashSaleStock.remaining` is unchanged
- **VERIFIED BY**: platform-e2e `features/order/advertised_price.feature` "A buyer cannot hold flash stock directly"
