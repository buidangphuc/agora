@analytics @batch @integration
Feature: Listing attributes become item and user features
  team-analytics keeps each listing's category and price from listing.events and exports them as
  listing_sellers.parquet; `python -m featurestore materialize` turns them into item_attributes@v1
  (per listing) and user_preferences@v1 (per user), with the offline snapshot, versioned online keys
  and the parity gate of every other view. Runs here as the real job image on the stack network,
  against the real analytics export and, for the point-in-time, parity and input-guard scenarios,
  against synthetic inputs built inside the image. Needs the rebuilt team-analytics and
  platform-featurestore images. (featurestore-item-attributes / item-attributes)

  The real-stack scenarios share one batch of activity (one seller, three listings, one buyer),
  created by the first scenario that runs, so the export cycle (300 s locally) is waited out once.

  Scenario: A listing's category and price reach the listing export
    When a seller creates a listing with a category and a price and the next export cycle completes
    Then listing_sellers.parquet on the analytics volume has a row for that listing with that category_id and price

  Scenario: An edited listing's attributes replace the old ones
    When the seller changes that listing's price and the next export cycle completes
    Then listing_sellers.parquet holds the new price for that listing and exactly one row for it

  Scenario: A listing's attributes become item features
    When a seller creates a listing with a category and a price, the export cycle completes, and the materialisation job runs
    Then the online features of that listing under item_attributes@v1 carry that category and price and the seller

  Scenario: A buyer's preferred categories follow their interactions
    When a buyer views a listing of category A three times and a listing of category B once, the export cycle completes, and the materialisation job runs
    Then the online preferred_categories of that buyer under user_preferences@v1 is "A,B"

  Scenario: A listing changed after AS_OF is not in the snapshot
    When the listing export holds one listing last changed before AS_OF and one changed after it, and the job runs
    Then the item_attributes@v1 snapshot has a row for the first and none for the second

  Scenario: A tampered attribute fails the parity check
    When after a run, one listing's item_attributes@v1 online category is overwritten with a different value, and python -m featurestore parity runs
    Then the command exits non-zero and its output names that listing and category_id

  Scenario: Without a listing export the attribute views are empty
    When the job runs over inputs without listing_sellers.parquet
    Then it exits 0 with a warning naming the file, and the item_attributes@v1 and user_preferences@v1 snapshots have no rows

  Scenario: A listing export without the new columns stops the run
    When the job runs over a listing_sellers.parquet that has no category_id column
    Then it exits 2 and its output names category_id
