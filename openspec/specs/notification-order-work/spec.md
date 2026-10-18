# notification-order-work Specification

## Purpose
team-notification tells buyers and sellers about order lifecycle work: new orders, payment reminders, expiry cancels, completion, refunds, returns and low stock.

## Requirements

### Requirement: A seller is notified once of a new order

`team-notification` SHALL create one `NOTIFICATION_TYPE_ORDER` notification titled "Đơn hàng mới" for the seller of an order, linking to `/seller/orders/{order_id}`, when it consumes an `OrderPlaced` event whose `payment_method` is `PAYMENT_METHOD_COD`, or an `OrderPaidEvent` event. An `OrderPlaced` event of any other payment method SHALL create no notification. The recipient SHALL be the event's `seller_id`; an event with an empty `seller_id` SHALL be skipped without a DLQ entry. The body SHALL name the order id, the amount and the `ship_by` date.

#### Scenario: Order work: A COD order notifies its seller at placement
- **WHEN** an `OrderPlaced` event with payment_method COD, seller "S1", total_amount 250000 and a ship_by is consumed
- **THEN** seller "S1" has exactly one notification titled "Đơn hàng mới" with link_url `/seller/orders/{order_id}` and a body containing 250000
- **VERIFIED BY**: team-notification `internal/consumer` `TestOrderPlacedCODNotifiesSeller`

#### Scenario: Order work: An online order notifies its seller at payment
- **WHEN** an `OrderPaidEvent` with seller_id "S1" and a ship_by is consumed
- **THEN** seller "S1" has exactly one notification titled "Đơn hàng mới" with link_url `/seller/orders/{order_id}`
- **VERIFIED BY**: team-notification `internal/consumer` `TestOrderPaidNotifiesSeller`

#### Scenario: Order work: Another seller receives no order notification
- **WHEN** an `OrderPlaced` COD event for seller "S1" is consumed
- **THEN** another seller "S2" has no notification, and the order's buyer has none
- **VERIFIED BY**: team-notification `internal/consumer` `TestOrderPlacedOtherSellerNotNotified`

#### Scenario: Order work: An online order is not notified at placement
- **WHEN** an `OrderPlaced` event with payment_method MOCK_MOMO is consumed
- **THEN** no notification is created for the seller or the buyer
- **VERIFIED BY**: team-notification `internal/consumer` `TestOrderPlacedOnlineCreatesNothing`

#### Scenario: Order work: A redelivered order event does not notify twice
- **WHEN** the same `OrderPaidEvent` envelope (same event_id) is delivered twice
- **THEN** the seller has exactly one notification
- **VERIFIED BY**: team-notification `internal/consumer` `TestOrderPaidRedeliveryNotifiesOnce`

#### Scenario: Order work: A paid event without seller_id is skipped
- **WHEN** an `OrderPaidEvent` with an empty seller_id (published before the field existed) is consumed
- **THEN** no notification is created, the handler returns no error and nothing is sent to the DLQ
- **VERIFIED BY**: team-notification `internal/consumer` `TestOrderPaidWithoutSellerSkipped`

#### Scenario: Order work: A malformed order payload is a permanent error
- **WHEN** an `OrderPlaced` envelope whose payload cannot be unmarshalled is consumed
- **THEN** the handler returns `ErrPermanent` and no notification is created
- **VERIFIED BY**: team-notification `internal/consumer` `TestOrderPlacedMalformedIsPermanent`

#### Scenario: Order work: A seller disabling ORDER notifications receives none
- **WHEN** an `OrderPaidEvent` is consumed for a seller whose preferences set the ORDER type off
- **THEN** no notification is created for that seller
- **VERIFIED BY**: team-notification `internal/consumer` `TestOrderPaidRespectsTypePreference`

#### Scenario: Order work: A COD order reaches the seller feed end to end
- **WHEN** a buyer places a COD order with a seller's listing through the gateway
- **THEN** the seller's `ListNotifications` returns one "Đơn hàng mới" notification whose link_url is `/seller/orders/{order_id}`, and the buyer's list has none for that order
- **VERIFIED BY**: platform-e2e `features/notification/order_work.feature` "Order work: A COD order reaches the seller feed end to end"

#### Scenario: Order work: A paid online order reaches the seller feed once end to end
- **WHEN** a buyer places an online-pay order and pays it through `ProcessMockPayment`
- **THEN** the seller's `ListNotifications` returns exactly one "Đơn hàng mới" notification for that order, and only after the payment, not at placement
- **VERIFIED BY**: platform-e2e `features/notification/order_work.feature` "Order work: A paid online order reaches the seller feed once end to end"

### Requirement: A buyer is notified of the payment reminder and the expiry cancel

`team-notification` SHALL create one `NOTIFICATION_TYPE_ORDER` notification for `buyer_id` when it consumes an `OrderPaymentReminder` event, titled "Đơn hàng sắp hết hạn thanh toán", linking to `/checkout/pay/{order_id}`, with a body naming the order id, the amount and `payment_due_at`. It SHALL create one for `buyer_id` when it consumes an `OrderCancelled` event whose `reason_code` is `CANCEL_REASON_PAYMENT_EXPIRED`, titled "Đơn hàng đã bị hủy do quá hạn thanh toán", linking to `/account/orders/{order_id}`. An `OrderCancelled` event with any other `reason_code` (including unspecified) SHALL create no notification. An event of this requirement with an empty `buyer_id` SHALL be skipped without a DLQ entry.

#### Scenario: Order work: A payment reminder notifies the buyer with a pay link
- **WHEN** an `OrderPaymentReminder` event for buyer "B1" is consumed
- **THEN** buyer "B1" has exactly one notification titled "Đơn hàng sắp hết hạn thanh toán" with link_url `/checkout/pay/{order_id}`
- **VERIFIED BY**: team-notification `internal/consumer` `TestPaymentReminderNotifiesBuyer`

#### Scenario: Order work: An expiry cancel notifies the buyer
- **WHEN** an `OrderCancelled` event with reason_code PAYMENT_EXPIRED for buyer "B1" is consumed
- **THEN** buyer "B1" has exactly one notification titled "Đơn hàng đã bị hủy do quá hạn thanh toán" with link_url `/account/orders/{order_id}`
- **VERIFIED BY**: team-notification `internal/consumer` `TestCancelExpiredNotifiesBuyer`

#### Scenario: Order work: A buyer's own cancel creates no notification
- **WHEN** an `OrderCancelled` event with reason_code CHANGED_MIND, and another with reason_code unspecified, are consumed
- **THEN** no notification is created for the buyer or the seller
- **VERIFIED BY**: team-notification `internal/consumer` `TestCancelOtherReasonCreatesNothing`

#### Scenario: Order work: A reminder without buyer_id is skipped
- **WHEN** an `OrderPaymentReminder` event with an empty buyer_id is consumed
- **THEN** no notification is created, the handler returns no error and nothing is sent to the DLQ
- **VERIFIED BY**: team-notification `internal/consumer` `TestPaymentReminderMissingBuyerSkipped`

#### Scenario: Order work: A reminder is created for the buyer only
- **WHEN** an `OrderPaymentReminder` event for buyer "B1" and seller "S1"'s order is consumed
- **THEN** no notification exists for seller "S1" and none for any other buyer
- **VERIFIED BY**: team-notification `internal/consumer` `TestPaymentReminderOnlyRecipientIsBuyer`

#### Scenario: Order work: An unpaid order reminds and then expires its buyer end to end
- **WHEN** a buyer places an online-pay order and does not pay it while the e2e overlay's shortened payment timers elapse
- **THEN** the buyer's `ListNotifications` returns one payment-reminder notification linking to `/checkout/pay/{order_id}`, and later one expiry-cancel notification for the same order
- **VERIFIED BY**: platform-e2e `features/notification/order_work.feature` "Order work: An unpaid order reminds and then expires its buyer end to end"

### Requirement: A buyer is notified of completion and refund; a seller of a return request

`team-notification` SHALL create one `NOTIFICATION_TYPE_ORDER` notification for `buyer_id` when it consumes an `OrderCompleted` event, whatever its `completed_by`, titled "Đơn hàng đã hoàn tất", with a body that prompts a review and names the `return_window_ends_at` date, linking to `/account/orders/{order_id}`. It SHALL create one for `seller_id` when it consumes a `ReturnRequested` event, titled "Yêu cầu trả hàng mới", linking to `/seller/orders/{order_id}?tab=returns`. It SHALL create one for `buyer_id` when it consumes a `ReturnRefunded` event, titled "Yêu cầu trả hàng đã được hoàn tiền", body naming `refund_amount`, linking to `/account/orders/{order_id}`.

#### Scenario: Order work: A completed order prompts the buyer to review
- **WHEN** an `OrderCompleted` event with completed_by SYSTEM for buyer "B1" is consumed
- **THEN** buyer "B1" has exactly one notification titled "Đơn hàng đã hoàn tất" with link_url `/account/orders/{order_id}` and a body that contains the return window end date
- **VERIFIED BY**: team-notification `internal/consumer` `TestOrderCompletedNotifiesBuyer`

#### Scenario: Order work: Completion by the buyer or the seller notifies the buyer too
- **WHEN** `OrderCompleted` events with completed_by BUYER and completed_by SELLER (different orders) are consumed
- **THEN** the buyer has one "Đơn hàng đã hoàn tất" notification per order and the seller has none
- **VERIFIED BY**: team-notification `internal/consumer` `TestOrderCompletedAnyActorNotifiesBuyerOnly`

#### Scenario: Order work: A return request notifies the seller with the returns link
- **WHEN** a `ReturnRequested` event for seller "S1" and order "O1" is consumed
- **THEN** seller "S1" has exactly one notification titled "Yêu cầu trả hàng mới" with link_url `/seller/orders/O1?tab=returns`
- **VERIFIED BY**: team-notification `internal/consumer` `TestReturnRequestedNotifiesSeller`

#### Scenario: Order work: A refunded return notifies the buyer
- **WHEN** a `ReturnRefunded` event with refund_amount 120000 for buyer "B1" is consumed
- **THEN** buyer "B1" has exactly one notification titled "Yêu cầu trả hàng đã được hoàn tiền" whose body contains 120000
- **VERIFIED BY**: team-notification `internal/consumer` `TestReturnRefundedNotifiesBuyer`

#### Scenario: Order work: A redelivered return request does not notify twice
- **WHEN** the same `ReturnRequested` envelope is delivered twice
- **THEN** the seller has exactly one notification
- **VERIFIED BY**: team-notification `internal/consumer` `TestReturnRequestedRedeliveryNotifiesOnce`

#### Scenario: Order work: A notification failure leaves the event unrecorded
- **WHEN** creating the notification fails with a database error while an `OrderCompleted` event is consumed, and the event is redelivered after the database recovers
- **THEN** the first call returns a non-permanent error and records nothing in the ledger, and the redelivery creates exactly one notification
- **VERIFIED BY**: team-notification `internal/consumer` `TestOrderCompletedTransientFailureRetries`

#### Scenario: Order work: A return request reaches the seller feed end to end
- **WHEN** a buyer creates a return on a shipped order through the gateway
- **THEN** the seller's `ListNotifications` returns one "Yêu cầu trả hàng mới" notification whose link_url ends with `?tab=returns`
- **VERIFIED BY**: platform-e2e `features/notification/order_work.feature` "Order work: A return request reaches the seller feed end to end"

#### Scenario: Order work: A completed order reaches the buyer feed end to end
- **WHEN** a buyer confirms receipt of a shipped order through `ConfirmReceipt`
- **THEN** the buyer's `ListNotifications` returns one "Đơn hàng đã hoàn tất" notification for that order, and another user's list does not contain it
- **VERIFIED BY**: platform-e2e `features/notification/order_work.feature` "Order work: A completed order reaches the buyer feed end to end"

### Requirement: A seller is notified once per downward crossing of the low-stock threshold

`team-notification` SHALL record `seller_id`, `title` and a `low` flag per listing in durable state: `ListingChanged` (not DELETED) SHALL upsert `seller_id` and `title`, and SHALL set `low` to `snapshot stock <= LOW_STOCK_THRESHOLD` on every snapshot, without notifying. `ListingChanged` with change_type DELETED SHALL delete the listing's state, without notifying. When it consumes `ListingStockChanged` for a listing with state, it SHALL compare `stock <= LOW_STOCK_THRESHOLD` (default 5) with `low`: on false to true it SHALL create one `NOTIFICATION_TYPE_SYSTEM` notification for the stored `seller_id`, titled "Sắp hết hàng", body naming the listing title and the stock, linking to `/seller/{listing_id}/edit`, and then set `low`; on true to false it SHALL clear `low` without notifying. A `ListingStockChanged` for a listing with no state SHALL create no notification and store nothing. `low` SHALL be advanced only after the notification succeeded.

#### Scenario: Order work: Stock falling to the threshold notifies the seller once
- **WHEN** listing "L1" of seller "S1" is known with stock 8 and a `ListingStockChanged` with stock 5 is consumed
- **THEN** seller "S1" has exactly one "Sắp hết hàng" notification with link_url `/seller/L1/edit`, and its body contains the listing title and 5
- **VERIFIED BY**: team-notification `internal/consumer` `TestLowStockCrossingNotifiesSeller`

#### Scenario: Order work: Stock staying at or below the threshold does not repeat
- **WHEN** after the crossing to 5, `ListingStockChanged` events with stock 4 and then 2 are consumed
- **THEN** the seller still has exactly one low-stock notification
- **VERIFIED BY**: team-notification `internal/consumer` `TestLowStockNoRepeatWhileLow`

#### Scenario: Order work: A fall after a rise above the threshold notifies again
- **WHEN** after the crossing, stock rises to 6 (event consumed) and then falls to 3 (event consumed)
- **THEN** the seller has exactly two low-stock notifications
- **VERIFIED BY**: team-notification `internal/consumer` `TestLowStockRearmsAfterRiseAboveThreshold`

#### Scenario: Order work: Stock above the threshold creates nothing
- **WHEN** listing "L1" is known with stock 20 and `ListingStockChanged` events with stock 12 and 6 are consumed
- **THEN** no low-stock notification exists for the seller
- **VERIFIED BY**: team-notification `internal/consumer` `TestLowStockAboveThresholdCreatesNothing`

#### Scenario: Order work: A listing seen first with low stock does not notify
- **WHEN** a `ListingChanged` snapshot of a new listing with stock 3 is consumed
- **THEN** no low-stock notification is created, and a later `ListingStockChanged` with stock 2 creates none either
- **VERIFIED BY**: team-notification `internal/consumer` `TestLowStockSeedLowDoesNotNotify`

#### Scenario: Order work: A restock through a listing edit re-arms low stock
- **WHEN** listing "L1" crossed to stock 3 (`low` true, one notification), then a `ListingChanged` snapshot with stock 20 is consumed, then a `ListingStockChanged` with stock 4 is consumed
- **THEN** the `ListingChanged` creates no notification and clears `low`, and the seller has exactly two low-stock notifications
- **VERIFIED BY**: team-notification `internal/consumer` `TestLowStockRearmsOnListingChangedRestock`

#### Scenario: Order work: A deleted listing drops its low-stock state
- **WHEN** a `ListingChanged` with change_type DELETED is consumed for known listing "L1", then a `ListingStockChanged` with stock 1 is consumed for "L1"
- **THEN** the state row for "L1" no longer exists, no notification is created and the handler returns no error for either event
- **VERIFIED BY**: team-notification `internal/consumer` `TestLowStockDeletedListingDropsState`; team-notification `internal/repository` `TestPostgresLowStockStoreDelete`

#### Scenario: Order work: A stock event for an unknown listing is ignored
- **WHEN** a `ListingStockChanged` with stock 1 is consumed for a listing with no stored state
- **THEN** no notification is created, no state row is stored and the handler returns no error
- **VERIFIED BY**: team-notification `internal/consumer` `TestLowStockUnknownListingIgnored`

#### Scenario: Order work: A failed low-stock notification re-detects on redelivery
- **WHEN** creating the low-stock notification fails with a database error, and the event is redelivered after recovery
- **THEN** the first call returns a non-permanent error with `low` still false, and the redelivery creates exactly one notification
- **VERIFIED BY**: team-notification `internal/consumer` `TestLowStockFailureRedetectsOnRedelivery`

#### Scenario: Order work: The notification goes to the listing's owner only
- **WHEN** seller "S1"'s listing "L1" crosses the threshold while another seller "S2" owns a listing "L2" above it
- **THEN** only seller "S1" has a low-stock notification and the other seller "S2" has none
- **VERIFIED BY**: team-notification `internal/consumer` `TestLowStockOnlyOwnerNotified`

#### Scenario: Order work: The threshold is configurable
- **WHEN** the consumer is built with LOW_STOCK_THRESHOLD 10 and a known listing falls from 12 to 10
- **THEN** the seller has exactly one low-stock notification
- **VERIFIED BY**: team-notification `internal/consumer` `TestLowStockConfiguredThreshold`; team-notification `internal/config` `TestLoadLowStockThreshold`

#### Scenario: Order work: An invalid threshold value fails startup
- **WHEN** `LOW_STOCK_THRESHOLD` is set to "-1" or "abc"
- **THEN** config loading returns an error and the service does not start
- **VERIFIED BY**: team-notification `internal/config` `TestLoadLowStockThresholdInvalid`

#### Scenario: Order work: Low-stock state survives a restart
- **WHEN** a crossing was recorded (`low` true) and the consumer restarts with the Postgres store, then a `ListingStockChanged` with stock 4 is consumed
- **THEN** no second notification is created
- **VERIFIED BY**: team-notification `internal/consumer` `TestLowStockStateDurableAcrossRestart`

#### Scenario: Order work: Selling stock down notifies the seller once end to end
- **WHEN** a seller's listing with stock 6 is bought down by a buyer's order of 2 units (stock 4) and then of 1 unit (stock 3)
- **THEN** the seller's `ListNotifications` returns exactly one "Sắp hết hàng" notification for that listing
- **VERIFIED BY**: platform-e2e `features/notification/order_work.feature` "Order work: Selling stock down notifies the seller once end to end"
