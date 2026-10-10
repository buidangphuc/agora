# item-attributes Specification

## Purpose
Defines the listing attribute export from the warehouse and the item_attributes and user_preferences feature views materialised from it.

## Requirements

### Requirement: The warehouse exports listing attributes

team-analytics SHALL keep the `category_id` and `price` (minor units) of every listing it has seen on `listing.events`
next to its seller, the newest event winning, and the Parquet export SHALL write `listing_sellers.parquet` beside the
other feature inputs on every export cycle (columns `listing_id`, `seller_id`, `updated_at`, `category_id`, `price`),
replaced atomically. A deleted listing SHALL keep its row. Rows written before these columns existed SHALL be refreshed
by replaying `listing.events`.

#### Scenario: A listing's category and price reach the listing export

- **WHEN** a seller creates a listing with a category and a price and the next export cycle completes
- **THEN** `listing_sellers.parquet` on the analytics volume has a row for that listing with that `category_id` and `price`

#### Scenario: An edited listing's attributes replace the old ones

- **WHEN** the seller changes that listing's price and the next export cycle completes
- **THEN** `listing_sellers.parquet` holds the new price for that listing and exactly one row for it

### Requirement: Attribute views are materialised from the listing export

The registry SHALL declare `item_attributes@v1` and `user_preferences@v1`, computed as of `AS_OF` and written to the
offline snapshot and to versioned online keys with the same manifest, freshness and parity rules as the other views.
- `item_attributes@v1`, per listing: `seller_id`, `category_id`, `price`. A listing whose latest recorded change is after
  `AS_OF` SHALL NOT appear. An attribute that is unknown is null, never a substituted constant.
- `user_preferences@v1`, per `user_key`: `preferred_categories`, the up to three categories with the highest weighted
  count of the user's views (1), clicks (2) and add-to-carts (5) on listings with a known category in the 30 days before
  `AS_OF`, ordered by weight descending then name, joined by a comma. A user with no such interaction has no row.

When `listing_sellers.parquet` does not exist the job SHALL warn and both views SHALL be empty (the other views are
unaffected). When it exists without `category_id` or `price` the job SHALL exit 2 naming the column.

#### Scenario: A listing's attributes become item features

- **WHEN** a seller creates a listing with a category and a price, the export cycle completes, and the materialisation
  job runs
- **THEN** the online features of that listing under item_attributes@v1 carry that category and price and the seller

#### Scenario: A buyer's preferred categories follow their interactions

- **WHEN** a buyer views a listing of category A three times and a listing of category B once, the export cycle completes,
  and the materialisation job runs
- **THEN** the online preferred_categories of that buyer under user_preferences@v1 is "A,B"

#### Scenario: A listing changed after AS_OF is not in the snapshot

- **WHEN** the listing export holds one listing last changed before AS_OF and one changed after it, and the job runs
- **THEN** the item_attributes@v1 snapshot has a row for the first and none for the second

#### Scenario: A tampered attribute fails the parity check

- **WHEN** after a run, one listing's item_attributes@v1 online category is overwritten with a different value, and
  python -m featurestore parity runs
- **THEN** the command exits non-zero and its output names that listing and category_id

#### Scenario: Without a listing export the attribute views are empty

- **WHEN** the job runs over inputs without listing_sellers.parquet
- **THEN** it exits 0 with a warning naming the file, and the item_attributes@v1 and user_preferences@v1 snapshots have no rows

#### Scenario: A listing export without the new columns stops the run

- **WHEN** the job runs over a listing_sellers.parquet that has no category_id column
- **THEN** it exits 2 and its output names category_id
