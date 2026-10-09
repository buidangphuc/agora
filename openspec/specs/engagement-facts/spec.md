# engagement-facts Specification

## Purpose
Defines which engagement state changes (favourites, follows, review ratings) team-engagement publishes as facts
through its transactional outbox, and how team-analytics stores them and derives the current state.

## Requirements

### Requirement: Engagement state changes are published as facts

team-engagement SHALL write a fact in the same transaction as each of these state changes:
- `FavoriteAdded` for `AddFavorite` and `FavoriteRemoved` for `RemoveFavorite`, both carrying user and listing;
- `SellerFollowed` for `FollowSeller` and `SellerUnfollowed` for `UnfollowSeller`, both carrying user and seller;
- `ReviewCreated` for `CreateReview`, carrying user, listing, seller and rating, and no review text.

A call that changes no state SHALL write no fact, for example adding an existing favourite or removing a missing one.
Facts SHALL be published in commit order to `engagement.events` inside the standard `EventEnvelope`. The envelope's
principal SHALL be the caller and its `type` SHALL be the payload's full name. The record key SHALL be the listing id, or
the seller id for follows. A fact SHALL be published at least once, even if Kafka is down when it is written.

#### Scenario: Favouriting a listing publishes a fact

- **WHEN** a logged-in buyer adds a listing to their favourites through the gateway
- **THEN** one `FavoriteAdded` envelope for that buyer and listing appears on `engagement.events`

#### Scenario: Removing a favourite publishes a removal

- **WHEN** the buyer then removes that favourite
- **THEN** a `FavoriteRemoved` envelope for that buyer and listing follows it on `engagement.events`

#### Scenario: A repeated favourite publishes nothing new

- **WHEN** a buyer adds the same listing to their favourites twice
- **THEN** exactly one `FavoriteAdded` envelope for that buyer and listing appears on `engagement.events`

#### Scenario: A review publishes its rating without its text

- **WHEN** a buyer with a delivered order reviews the listing with rating 4 and the text "rất tốt"
- **THEN** a `ReviewCreated` envelope with rating 4, the listing and its seller appears on `engagement.events`, and the
  payload does not contain "rất tốt"

#### Scenario: A fact written while Kafka is down is published later

- **WHEN** Redpanda is stopped, a buyer follows a seller, and Redpanda is started again
- **THEN** one `SellerFollowed` envelope for that buyer and seller appears on `engagement.events`

### Requirement: The warehouse stores engagement facts and their current state

team-analytics SHALL consume `engagement.events` into `engagement_facts`, keeping one row per `event_id`, with `fact`,
`user_id`, `listing_id`, `seller_id`, `rating`, `occurred_at` and `ingested_at`. An undecodable record SHALL go to
`engagement.events.analytics.dlq` and SHALL NOT stop consumption. The view `favorites_current` SHALL list the
(user, listing) pairs whose latest favourite fact is an add. The view `follows_current` SHALL list the (user, seller)
pairs whose latest follow fact is a follow.

#### Scenario: A favourite that was removed is not current

- **WHEN** a buyer adds two listings to their favourites and then removes the first one
- **THEN** `engagement_facts` holds three rows for the buyer, and `favorites_current` lists only the second listing for
  the buyer

#### Scenario: A follow appears in the current follows

- **WHEN** a buyer follows a seller
- **THEN** `follows_current` lists that buyer and seller

#### Scenario: A malformed engagement record is dead-lettered

- **WHEN** a malformed record is produced to `engagement.events`, followed by a buyer favouriting a listing
- **THEN** the malformed record appears on `engagement.events.analytics.dlq`, and the favourite still reaches
  `engagement_facts`
