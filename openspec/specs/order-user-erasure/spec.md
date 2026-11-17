# order-user-erasure Specification

## Purpose
TBD - created by archiving change commerce-user-erasure. Update Purpose after archive.

## Requirements

### Requirement: team-order erases a user's contact data on UserErased

On `identity.events/UserErased` team-order SHALL, in one transaction and keyed only by the event's `user_id`,
replace `recipient_name`, `phone`, `street`, `ward`, `district` and `city` of `shipping_address` with "Đã xóa" on
every order whose `buyer_id` is the user, replace `reason` with "Đã xóa" on every return whose `buyer_id` is the
user, and delete the user's cart items. It SHALL keep order and return ids, buyer and seller ids, items, amounts,
statuses, shipments and sagas. It SHALL consume with its own consumer group and DLQ `identity.events.order.dlq`,
SHALL be idempotent by user id, and SHALL ignore every other event type on the topic.

#### Scenario: ORD-1 Buyer contact data is masked and accounting data is kept
- **GIVEN** buyer U has 3 orders with recipient "Nguyễn Văn A", phone "0901234567" and a street address, one return with reason "Hàng bị vỡ", and 2 cart items
- **WHEN** a `UserErased` event for U is consumed
- **THEN** all 3 orders read recipient name, phone, street, ward, district and city "Đã xóa", the return reason reads "Đã xóa", U has 0 cart items, and order ids, `buyer_id`, `seller_id`, `total_amount`, items and shipments are byte-identical to before

#### Scenario: ORD-2 Another user's data is untouched
- **GIVEN** users U and W each have orders, returns and cart items
- **WHEN** a `UserErased` event for U is consumed
- **THEN** W's orders, returns and cart items are unchanged

#### Scenario: ORD-3 A redelivered event changes nothing
- **GIVEN** a `UserErased` event for U was consumed
- **WHEN** the same event, and then a second event with a new event id for U, are consumed
- **THEN** both complete without error, no row differs from after the first handling, and the DLQ stays empty

#### Scenario: ORD-4 A user with no data is a no-op
- **WHEN** a `UserErased` event for a user with no orders, returns or cart items is consumed
- **THEN** the handler completes OK and no row in any table changes
- **VERIFIED BY**: team-order/internal/store TestEraseUser

#### Scenario: ORD-5 A seller's orders keep the buyers' addresses
- **GIVEN** U is a seller (roles include seller) of an order placed by buyer B
- **WHEN** a `UserErased` event for U is consumed
- **THEN** B's recipient name, phone and address on that order are unchanged and the order still names U's id as `seller_id`
- **VERIFIED BY**: team-order/internal/store TestEraseUser

#### Scenario: ORD-6 A malformed event is parked, not applied
- **WHEN** a `UserErased` event with an empty `user_id`, or an envelope that does not decode, is consumed
- **THEN** no row changes and the record is written to `identity.events.order.dlq`

#### Scenario: ORD-7 A store error is retried and leaves no partial erasure
- **GIVEN** the order database fails after the cart delete but before the commit
- **WHEN** a `UserErased` event for U is consumed
- **THEN** the transaction rolls back (orders still unmasked), the record is retried, and after the database recovers all of U's data is masked and the DLQ is empty
- **VERIFIED BY**: team-order/internal/store TestEraseUserRollsBack

#### Scenario: ORD-8 Other identity events are ignored
- **WHEN** a `UserRegistered` or `UserStatusChanged` event for U is consumed
- **THEN** no order, return or cart row changes
- **VERIFIED BY**: team-order/internal/events TestErasureConsumerIgnoresOtherEvents

#### Scenario: ORD-9 The masked order stays readable by its parties
- **GIVEN** U's data was erased and a seller S owns one of U's orders
- **WHEN** S reads that order through `GetOrder`
- **THEN** the order returns with recipient name "Đã xóa", its original total and items, and the call succeeds
