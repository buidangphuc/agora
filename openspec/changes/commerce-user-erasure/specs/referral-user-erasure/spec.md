## ADDED Requirements

### Requirement: team-referral deletes a user's referral identity on UserErased

On `identity.events/UserErased` team-referral SHALL, in one transaction and keyed only by the event's `user_id`,
delete the user's rows in `referral_codes`, `referral_accounts` and `referral_buyer_orders`. It SHALL keep
`referrals` rows (referrer id, referee id, code, status) and the `referral_rewards` ledger with ids and amounts. It
SHALL consume with its own consumer group (separate from the existing identity and order consumers) and DLQ
`identity.events.referral.dlq`, SHALL be idempotent by user id, and SHALL ignore every other event type.

#### Scenario: REF-1 Code, account and first-order rows are deleted and the ledger is kept
- **GIVEN** user U has a referral code, a `referral_accounts` row, a `referral_buyer_orders` row, 2 referral attributions as referrer and 3 reward rows totalling 150 coins
- **WHEN** a `UserErased` event for U is consumed
- **THEN** U's code, account and first-order rows are gone, and the 2 attributions and the 3 reward rows (ids, amounts, status) are unchanged

#### Scenario: REF-2 A deleted code no longer resolves
- **GIVEN** U's referral code was `ABC123` before erasure
- **WHEN** a new user registers with code `ABC123` after U's erasure is consumed
- **THEN** the referral is not attributed to U and no `referrals` row is added for that code

#### Scenario: REF-3 Another user's referral data is untouched
- **GIVEN** user W has a referral code, account row and first-order row
- **WHEN** a `UserErased` event for U is consumed
- **THEN** W's three rows are unchanged

#### Scenario: REF-4 A redelivered event changes nothing
- **GIVEN** a `UserErased` event for U was consumed
- **WHEN** the same event, and then a second event with a new event id for U, are consumed
- **THEN** both complete without error, no row differs from after the first handling, and the DLQ stays empty

#### Scenario: REF-5 A user with no referral data is a no-op
- **WHEN** a `UserErased` event for a user with no referral rows is consumed
- **THEN** the handler completes OK and no row changes

#### Scenario: REF-6 A malformed event is parked, not applied
- **WHEN** a `UserErased` event with an empty `user_id`, or an envelope that does not decode, is consumed
- **THEN** no row changes and the record is written to `identity.events.referral.dlq`

#### Scenario: REF-7 A store error is retried and leaves no partial erasure
- **GIVEN** the referral database fails after the code delete but before the commit
- **WHEN** a `UserErased` event for U is consumed
- **THEN** the transaction rolls back (code still present), the record is retried, and after recovery all three rows are deleted and the DLQ is empty

#### Scenario: REF-8 The existing consumers ignore the new event
- **WHEN** a `UserErased` event reaches the existing `UserRegistered` identity consumer group
- **THEN** it is skipped without error and without writing a `referral_accounts` row
