## ADDED Requirements

### Requirement: team-payment masks a user's payout bank details on UserErased

On `identity.events/UserErased` team-payment SHALL, in one transaction and keyed only by the event's `user_id`,
replace `account_number` and `account_name` with "Đã xóa" on every payout request whose `seller_id` is the user,
whatever its status. It SHALL keep payout ids, `bank_code`, amounts, statuses, the wallet ledger and payment
transactions. It SHALL consume with its own consumer group and DLQ `identity.events.payment.dlq`, SHALL be
idempotent by user id, and SHALL ignore every other event type on the topic.

#### Scenario: PER-1 Bank details are masked and money records are kept
- **GIVEN** seller S has 3 payout requests (PENDING, COMPLETED, REJECTED) with account number "0123456789" and account name "NGUYEN VAN A", and ledger entries summing to 400000
- **WHEN** a `UserErased` event for S is consumed
- **THEN** the 3 payout requests read `account_number` and `account_name` "Đã xóa", and their ids, `bank_code`, `amount`, `status`, `ledger_entry_id` and S's ledger sum of 400000 are unchanged

#### Scenario: PER-2 Another seller's payouts are untouched
- **GIVEN** sellers S and T each have payout requests
- **WHEN** a `UserErased` event for S is consumed
- **THEN** T's `account_number` and `account_name` are unchanged

#### Scenario: PER-3 A redelivered event changes nothing
- **GIVEN** a `UserErased` event for S was consumed
- **WHEN** the same event, and then a second event with a new event id for S, are consumed
- **THEN** both complete without error, no row differs from after the first handling, and the DLQ stays empty

#### Scenario: PER-4 A user without payouts is a no-op
- **WHEN** a `UserErased` event for a buyer with payment transactions but no payout requests is consumed
- **THEN** the handler completes OK and no payment transaction or ledger row changes
- **VERIFIED BY**: team-payment/internal/store TestEraseSellerPayouts

#### Scenario: PER-5 A malformed event is parked, not applied
- **WHEN** a `UserErased` event with an empty `user_id`, or an envelope that does not decode, is consumed
- **THEN** no row changes and the record is written to `identity.events.payment.dlq`
- **VERIFIED BY**: team-payment/internal/app TestErasureConsumerParksMalformed

#### Scenario: PER-6 A store error is retried and leaves no partial erasure
- **GIVEN** the payment database fails during the update
- **WHEN** a `UserErased` event for S is consumed
- **THEN** no payout request is masked until the database recovers, the record is retried, and afterwards all of S's payout requests are masked and the DLQ is empty
- **VERIFIED BY**: team-payment/internal/store TestEraseSellerPayoutsRollsBack

#### Scenario: PER-7 Other identity events are ignored
- **WHEN** a `UserRegistered` or `UserStatusChanged` event for S is consumed
- **THEN** no payout request changes
- **VERIFIED BY**: team-payment/internal/app TestErasureConsumerIgnoresOtherEvents

#### Scenario: PER-8 Payout history shows the mask
- **GIVEN** S's data was erased
- **WHEN** an admin lists S's payout history
- **THEN** each entry shows `account_number` and `account_name` "Đã xóa" with its original amount and status
