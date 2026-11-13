## ADDED Requirements

### Requirement: team-promotion releases a user's vouchers and state on UserErased

On `identity.events/UserErased` team-promotion SHALL, in one transaction and keyed only by the event's `user_id`,
deactivate every voucher whose `owner_user_id` is the user (setting `active` false and `deactivated_at`) and delete
the user's `new_user_voucher_state` row. It SHALL keep voucher reservations and redemptions with buyer id and
discount amount, and all vouchers not owned by the user. It SHALL consume with its own consumer group, SHALL write
malformed records to `identity.events.promotion.dlq`, SHALL be idempotent by user id, and SHALL ignore every other
event type.

#### Scenario: PRO-1 Owned vouchers are deactivated and state is deleted
- **GIVEN** user U owns 1 new-user voucher (kind 2, active), has a `new_user_voucher_state` row, and 2 committed reservations of 20000 and 30000
- **WHEN** a `UserErased` event for U is consumed
- **THEN** the voucher has `active` false and a `deactivated_at`, U's state row is gone, and the 2 reservations keep their buyer id, `status` and `discount_amount`

#### Scenario: PRO-2 Other users' vouchers and state are untouched
- **GIVEN** user W owns an active voucher and has a state row, and a shop voucher with `seller_id` U's id and no `owner_user_id` exists
- **WHEN** a `UserErased` event for U is consumed
- **THEN** W's voucher and state row are unchanged and the shop voucher is unchanged

#### Scenario: PRO-3 A redelivered event changes nothing
- **GIVEN** a `UserErased` event for U was consumed
- **WHEN** the same event, and then a second event with a new event id for U, are consumed
- **THEN** both complete without error, `deactivated_at` keeps its first value, and the DLQ stays empty

#### Scenario: PRO-4 A user with no promotion data is a no-op
- **WHEN** a `UserErased` event for a user who owns no voucher and has no state row is consumed
- **THEN** the handler completes OK and no row changes

#### Scenario: PRO-5 A malformed event is parked, not applied
- **WHEN** a `UserErased` event with an empty `user_id`, or an envelope that does not decode, is consumed
- **THEN** no row changes and the record is written to `identity.events.promotion.dlq`

#### Scenario: PRO-6 A store error is retried and leaves no partial erasure
- **GIVEN** the promotion database fails after the voucher update but before the commit
- **WHEN** a `UserErased` event for U is consumed
- **THEN** the transaction rolls back (voucher still active, state row still present), the record is retried, and after recovery both are applied and the DLQ is empty

#### Scenario: PRO-7 The existing identity consumer is not disturbed
- **WHEN** a `UserRegistered` event is consumed after the erasure consumer was added
- **THEN** the new-user voucher flow behaves as before (state row written) and the erasure consumer ignores that event

#### Scenario: PRO-8 A deactivated voucher cannot be redeemed
- **GIVEN** U's owned voucher was deactivated by erasure
- **WHEN** any caller reserves that voucher code at checkout
- **THEN** the reservation is refused as an inactive voucher and no quota is consumed

#### Scenario: PRO-9 A voucher owner cannot trigger erasure through the API
- **WHEN** a signed-in USER principal calls any `VoucherService` RPC naming another user's id, or a `UserErased` envelope is produced to `promotion.events`
- **THEN** the voucher API denies the call with `PERMISSION_DENIED` where it is routed, nothing is erased, and only records read from `identity.events` ever drive erasure
