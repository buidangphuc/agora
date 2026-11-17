# engagement-user-erasure Specification

## Purpose
TBD - created by archiving change content-user-erasure. Update Purpose after archive.

## Requirements

### Requirement: Reviews and Q&A keep their content and lose their author

On `identity.events/UserErased` team-engagement SHALL keep the rating and text of the user's reviews, questions and
answers, SHALL show the author as "Người dùng đã xóa", SHALL clear `hide_reason` on the user's reviews, and SHALL clear
`hidden_by` where it equals the user.

#### Scenario: A review keeps rating and text under an anonymised author

- **WHEN** a `UserErased` event for a user with a 4-star review "Giao nhanh" is consumed
- **THEN** the review is still returned by `ListReviews` with rating 4 and comment "Giao nhanh" and author name "Người dùng đã xóa"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/engagement/user_erasure.feature` `A review keeps rating and text under an anonymised author`

#### Scenario: A question and its answers keep their text under an anonymised author

- **WHEN** a `UserErased` event for a user who asked one question and answered another is consumed
- **THEN** both texts are still listed on the product page and neither row carries the erased user id
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/engagement/user_erasure.feature` `A question and its answers keep their text under an anonymised author`

#### Scenario: The listing rating is unchanged

- **WHEN** a `UserErased` event for the author of one of three reviews of a listing is consumed
- **THEN** the listing's review count stays 3 and its average rating is the same as before
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/engagement/user_erasure.feature` `The listing rating is unchanged`

#### Scenario: The moderation reason of an erased author is cleared

- **WHEN** a `UserErased` event is consumed for the author of a review an admin had hidden with a reason
- **THEN** the review's `hide_reason` is empty, `hidden_at` is still set and the review is still hidden from buyers
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/engagement/user_erasure.feature` `The moderation reason of an erased author is cleared`

#### Scenario: An erased moderator no longer appears as hidden_by

- **WHEN** a `UserErased` event is consumed for an admin who hid two reviews written by other users
- **THEN** `hidden_by` is empty on both reviews, `hide_reason` of those reviews is unchanged and both are still hidden
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/engagement/user_erasure.feature` `An erased moderator no longer appears as hidden_by`

#### Scenario: Other users' content is untouched

- **WHEN** a `UserErased` event for user A is consumed and user B has a review, a question and a favourite
- **THEN** every row of user B is byte-identical to its state before the event
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/engagement/user_erasure.feature` `Other users' content is untouched`

#### Scenario: Erasure is reachable only through the identity topic

- **WHEN** a `UserErased` record is produced to a topic other than `identity.events`, or a user principal calls any team-engagement RPC naming another user
- **THEN** no data of the named user is erased or changed
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/engagement/user_erasure.feature` `Erasure is reachable only through the identity topic`

### Requirement: Personal activity data is deleted

On `identity.events/UserErased` team-engagement SHALL delete the user's favourites, follows (as follower and as
followed seller), reports filed by the user, helpful votes, wishlist collections and items, view history, check-ins
and loyalty account.

#### Scenario: Favourites, follows and reports filed by the user are deleted

- **WHEN** a `UserErased` event is consumed for a user with 2 favourites, 1 follow, 1 report filed and 1 helpful vote
- **THEN** zero rows keyed by that user remain in `favorites`, `follows`, `reports` (as `reporter_id`) and `review_helpful_votes`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/engagement/user_erasure.feature` `Favourites, follows and reports filed by the user are deleted`

#### Scenario: An erased seller loses their followers

- **WHEN** a `UserErased` event is consumed for a seller followed by 3 buyers
- **THEN** zero `follows` rows with that `seller_id` remain and each follower's other follows are unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/engagement/user_erasure.feature` `An erased seller loses their followers`

#### Scenario: Collections, history, check-ins and loyalty are deleted

- **WHEN** a `UserErased` event is consumed for a user with a collection of 2 items, 5 viewed listings, 3 check-ins and a loyalty account
- **THEN** zero rows keyed by that user remain in `collections`, `collection_items`, `view_history`, `checkins` and `loyalty_accounts`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/engagement/user_erasure.feature` `Collections, history, check-ins and loyalty are deleted`

#### Scenario: Reports about the erased user's content stay open

- **WHEN** a `UserErased` event is consumed for the author of a listing review that another user reported
- **THEN** that report row still exists with its status unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/engagement/user_erasure.feature` `Reports about the erased user's content stay open`

#### Scenario: An event without a user id deletes nothing

- **WHEN** a `UserErased` event with an empty `user_id` is consumed
- **THEN** it is rejected to the DLQ and no favourite, follow, report or loyalty row of any user is deleted
- **VERIFIED BY**: `team-engagement/internal/events/identity_test.go` `TestIdentityEmptyUserIDDeletesNothing`

### Requirement: The engagement erasure consumer is idempotent and fail-visible

The team-engagement consumer SHALL use its own consumer group and the DLQ `identity.events.engagement.dlq`, SHALL apply
each event in one transaction, SHALL skip event types other than `UserErased`, and SHALL produce the same end state
on replay.

#### Scenario: A replayed event changes nothing

- **WHEN** the same `UserErased` event is delivered twice
- **THEN** the second delivery returns no error and the table contents equal the state after the first delivery
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/engagement/user_erasure.feature` `A replayed event changes nothing`

#### Scenario: A malformed event goes to the DLQ

- **WHEN** a record on `identity.events` is not a valid `EventEnvelope`, or a `UserErased` payload has an empty `user_id`
- **THEN** the record is published to `identity.events.engagement.dlq` without retry and no row changes
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/engagement/user_erasure.feature` `A malformed event goes to the DLQ`

#### Scenario: A store failure rolls back and is retried

- **WHEN** the erase transaction fails half-way because Postgres is unavailable
- **THEN** no table keeps a partial change, the record is retried up to 5 times, and after the database returns the erasure completes with the DLQ empty
- **VERIFIED BY**: `team-engagement/internal/store/erase_pg_test.go` `TestEraseRollsBackOnFailure`

#### Scenario: Other identity events are ignored

- **WHEN** a `UserStatusChanged` event for a user is consumed
- **THEN** no row of that user changes and nothing is published to the DLQ
- **VERIFIED BY**: `team-engagement/internal/events/identity_test.go` `TestIdentityIgnoresOtherEventTypes`

#### Scenario: Erasure completes within one hour

- **WHEN** a `UserErased` event is published to `identity.events` for a user holding 1000 rows across the tables
- **THEN** every row is erased or anonymised within 1 h and the store transaction takes at most 5 s
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/engagement/user_erasure.feature` `Erasure completes within one hour`

#### Scenario: The erased id is not logged

- **WHEN** the consumer handles a `UserErased` event
- **THEN** the service log holds a count and a hashed id and no line contains the raw user id
- **VERIFIED BY**: `team-engagement/internal/events/identity_test.go` `TestIdentityErasureLogsNoRawUserID`
