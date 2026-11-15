@analytics @integration
Feature: A UserErased event purges the user from the analytics warehouse
  team-analytics consumes `identity.events` in its own consumer group and, for a
  `platform.identity.v1.UserErased`, re-keys the user's rows in tracking_events, order_facts,
  engagement_facts and listing_sellers to a random `erased:<32 hex>` pseudonym, deletes favourite and
  follow facts, and keeps a keyed tombstone so late events are re-keyed on ingest. Poison records go to
  `identity.events.analytics.dlq`. Users are real registered accounts; rows are seeded through the
  gateway and observed in read-only copies of the DuckDB warehouse, the broker and the gateway's
  GetSellerFunnel RPC (GetPlatformOrderSummary is not routed by the gateway, so GMV is computed from order_facts). The consumer group defaults to
  team-analytics.erasure (ANALYTICS_ERASURE_GROUP overrides it).
  (analytics-user-erasure / analytics-warehouse-erasure)

  Scenario: AUE-1 A UserErased event purges the user
    Given a seller "S" with listings "L1", "L2" and "L3"
    And a buyer "U"
    And "U" has 5 tracking views of "L1" in the warehouse
    And "U" has a paid order of "L1", "L2" and "L3" in the warehouse
    And "U" has a favourite of "L1" in the warehouse
    When a UserErased event for "U" is produced
    Then within 60 seconds no warehouse table contains the id of "U"
    And the analytics erasure consumer group has no lag

  Scenario: AUE-2 Another identity event type is skipped
    Given a seller "S" with listings "L1", "L2" and "L3"
    And a buyer "U"
    And "U" has 5 tracking views of "L1" in the warehouse
    And the warehouse table sizes are recorded
    When a UserStatusChanged envelope for "U" is produced
    Then the analytics erasure consumer group has no lag
    And no warehouse table changed size
    And the id of "U" is still present in tracking_events
    And the analytics DLQ holds no record for that event

  Scenario: AUE-3 Poison record goes to the DLQ
    Given a buyer "U"
    And the warehouse table sizes are recorded
    When a record that is not an EventEnvelope and a UserErased with an empty user_id are produced
    Then both records are on identity.events.analytics.dlq with an error header
    And the analytics erasure consumer group has no lag
    And no warehouse table changed size

  Scenario: AUE-5 Redelivery is a no-op
    Given a seller "S" with listings "L1", "L2" and "L3"
    And a buyer "U"
    And "U" has 5 tracking views of "L1" in the warehouse
    When a UserErased event for "U" is produced
    And the analytics erasure consumer group has no lag
    And the warehouse table sizes are recorded
    And the same UserErased event for "U" is produced again
    Then the analytics erasure consumer group has no lag
    And no warehouse table changed size
    And the analytics DLQ holds no record for that event

  Scenario: AUE-6 Re-key keeps aggregates and removes identifiers
    Given a seller "S" with listings "L1", "L2" and "L3"
    And a buyer "U"
    And "U" has 50 tracking views of "L1" under anonymous ids "A1" and "A2", each also used by 10 anonymous views before login
    And the seller funnel of "S" is recorded
    When a UserErased event for "U" is produced
    Then within 60 seconds no warehouse table contains the id of "U", "A1" or "A2"
    And the 70 recorded tracking rows still exist and all carry the same single erased pseudonym
    And the seller funnel of "S" is unchanged

  Scenario: AUE-7 Rows of other users are untouched
    Given a seller "S" with listings "L1", "L2" and "L3"
    And a buyer "U"
    And a buyer "V"
    And "U" has 5 tracking views of "L1" in the warehouse
    And "V" has 20 tracking views of "L1" in the warehouse
    And the tracking rows of "V" are recorded
    When a UserErased event for "U" is produced
    Then within 60 seconds no warehouse table contains the id of "U"
    And the tracking rows of "V" are identical to the recorded ones

  Scenario: AUE-8 Boundary of a user with no rows
    Given a user "N" that has no rows in any table
    And the warehouse table sizes are recorded
    When a UserErased event for "N" is produced
    Then the analytics erasure consumer group has no lag
    And only erased_users grew, by one row
    And the analytics DLQ holds no record for that event

  Scenario: AUE-9 Orders are kept with a pseudonymous buyer
    Given a seller "S" with listings "L1", "L2" and "L3"
    And a buyer "U"
    And "U" has a paid order of "L1", "L2" and "L3" in the warehouse
    And the order lines of "U" and their order count and GMV are recorded
    When a UserErased event for "U" is produced
    Then within 60 seconds no warehouse table contains the id of "U"
    And the 3 recorded order lines keep order_id, quantity, unit_price and occurred_at and have an erased buyer_id
    And the order count and GMV of those orders are unchanged

  Scenario: AUE-10 Favourites and follows are deleted
    Given a seller "S" with 4 listings "L1" to "L4"
    And a second seller "S2"
    And a buyer "U"
    And a buyer "V"
    And "U" has 4 favourite facts and 2 follow facts in the warehouse
    And "V" has a favourite of "L1" in the warehouse
    When a UserErased event for "U" is produced
    Then within 60 seconds no warehouse table contains the id of "U"
    And engagement_facts, favorites_current and follows_current have no row for "U"
    And "V" still has 1 favourite fact and 1 current favourite

  Scenario: AUE-11 An erased seller is re-keyed everywhere
    Given a seller "U" with listings "L1", "L2" and "L3"
    And a buyer "B"
    And "B" has a paid order of "L1", "L2" and "L3" in the warehouse
    And "B" follows the seller "U" in the warehouse
    And a visitor has viewed "L1" and the seller funnel of "U" shows at least 1 view
    And the listing_sellers rows of "U" are recorded
    When a UserErased event for the seller "U" is produced
    Then within 60 seconds no warehouse table contains the id of "U"
    And the recorded listings keep their listing_id and status in listing_sellers
    And the seller funnel of "U" has zero counts

  Scenario: AUE-12 A buffered event flushed after the purge is re-keyed
    Given a seller "S" with listings "L1", "L2" and "L3"
    And a buyer "U"
    And "U" has 5 tracking views of "L1" in the warehouse
    When a UserErased event for "U" is produced
    And within 60 seconds no warehouse table contains the id of "U"
    And "U" posts a tracking view of "L1" with the token issued before the erasure
    Then the stored row of that view carries an erased pseudonym
    And within 60 seconds no warehouse table contains the id of "U"

  Scenario: AUE-13 The tombstone holds no clear id
    Given a seller "S" with listings "L1", "L2" and "L3"
    And a buyer "U"
    And "U" has 5 tracking views of "L1" in the warehouse
    When a UserErased event for "U" is produced
    And within 60 seconds no warehouse table contains the id of "U"
    Then erased_users has a row whose user_hmac is 64 hex characters
    And no column of erased_users equals the id of "U"

  Scenario: AUE-14 Exported files are clean after one cycle
    Given a seller "S" with listings "L1", "L2" and "L3"
    And a buyer "U"
    And "U" has 5 tracking views of "L1" under anonymous ids "A1" and "A2", each also used by 0 anonymous views before login
    And the Parquet export files contain the id of "U"
    When a UserErased event for "U" is produced
    Then after one export cycle a scan of the Parquet files for the id of "U", "A1" and "A2" finds zero matches

  Scenario: AUE-36 A forged event for another user does not touch unrelated rows
    Given a seller "S" with listings "L1", "L2" and "L3"
    And a buyer "V"
    And "V" has 5 tracking views of "L1" in the warehouse
    And the tracking rows of "V" are recorded
    When a UserErased event naming the id of "V" without its last character is produced
    Then the analytics erasure consumer group has no lag
    And the tracking rows of "V" are identical to the recorded ones

  Scenario: AUE-37 A late event for a non-erased user is stored as is
    Given a seller "S" with listings "L1", "L2" and "L3"
    And a buyer "U"
    And a buyer "V"
    And "U" has 5 tracking views of "L1" in the warehouse
    When a UserErased event for "U" is produced
    And within 60 seconds no warehouse table contains the id of "U"
    And the erased_users size is recorded
    And "V" posts a tracking view of "L1" with their own token
    Then the stored row of that view keeps the id of "V"
    And the erased_users size is unchanged
