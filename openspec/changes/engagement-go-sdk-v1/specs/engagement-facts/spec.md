## MODIFIED Requirements

### Requirement: Engagement state changes are published as facts

team-engagement SHALL write a fact in the same transaction as each of these state changes:
- `FavoriteAdded` for `AddFavorite` and `FavoriteRemoved` for `RemoveFavorite`, both carrying user and listing;
- `SellerFollowed` for `FollowSeller` and `SellerUnfollowed` for `UnfollowSeller`, both carrying user and seller;
- `ReviewCreated` for `CreateReview`, carrying user, listing, seller and rating, and no review text.

A call that changes no state SHALL write no fact, for example adding an existing favourite or removing a missing one.
Facts SHALL be published in write (`seq`) order for each record key (two concurrent transactions on one key can commit in the opposite order) to `engagement.events` inside the standard `EventEnvelope`;
facts with different keys carry no order guarantee between them. The envelope's principal SHALL be the caller and its
`type` SHALL be the payload's full name. The record key SHALL be the listing id, or the seller id for follows. A fact
SHALL be published at least once, even if Kafka is down when it is written, and a broker outage SHALL NOT park it.

#### Scenario: Favouriting a listing publishes a fact

- **WHEN** a logged-in buyer adds a listing to their favourites through the gateway
- **THEN** one `FavoriteAdded` envelope for that buyer and listing appears on `engagement.events`
- **VERIFIED BY**: team-engagement `internal/events` `TestRelayPublishesPerKeyInOrder`; e2e `engagement/engagement_facts.feature`

#### Scenario: Removing a favourite publishes a removal

- **WHEN** the buyer then removes that favourite
- **THEN** a `FavoriteRemoved` envelope for that buyer and listing follows it on `engagement.events`, after the `FavoriteAdded` of the same key
- **VERIFIED BY**: team-engagement `internal/events` `TestRelayPublishesPerKeyInOrder`; e2e `engagement/engagement_facts.feature`

#### Scenario: A repeated favourite publishes nothing new

- **WHEN** a buyer adds the same listing to their favourites twice
- **THEN** exactly one `FavoriteAdded` envelope for that buyer and listing appears on `engagement.events`
- **VERIFIED BY**: team-engagement `internal/store` `TestRepeatedFavouriteWritesOneFact`; e2e `engagement/engagement_facts.feature`

#### Scenario: A review publishes its rating without its text

- **WHEN** a buyer with a delivered order reviews the listing with rating 4 and the text "rất tốt"
- **THEN** a `ReviewCreated` envelope with rating 4, the listing and its seller appears on `engagement.events`, and the
  payload does not contain "rất tốt"
- **VERIFIED BY**: team-engagement `internal/events` `TestFactRowBuildsEnvelope`; e2e `engagement/engagement_facts.feature`

#### Scenario: A fact written while Kafka is down is published later

- **WHEN** Redpanda is stopped, a buyer follows a seller, and Redpanda is started again
- **THEN** one `SellerFollowed` envelope for that buyer and seller appears on `engagement.events` and its outbox row was never parked
- **VERIFIED BY**: team-engagement `internal/events` `TestRelayTransientErrorsNeverPark`; e2e `engagement/engagement_facts.feature`

#### Scenario: Facts of different keys carry no mutual order

- **WHEN** a buyer favourites listing `L1` and follows seller `S1` in two transactions
- **THEN** both envelopes appear on `engagement.events` exactly once, and only the order within key `L1` and within key `S1` is asserted
- **VERIFIED BY**: team-engagement `internal/events` `TestRelayOrderIsPerKeyOnly`
