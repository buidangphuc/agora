## MODIFIED Requirements

### Requirement: Tracking hooks are unchanged

The rework SHALL NOT change the tracking behaviour of discovery routes except that impressions are coalesced per list: every rendered `ListingGrid` SHALL announce its visible cards as one `view_item_list` per list instance (viewport threshold 0.3), with items in grid order, `index` 1..N, a shared `item_list_id` equal to the list's placement id and one shared event group id. `ListingCard` SHALL still link image and title through `TrackLink` (`select_item`); both the list impression items and the click SHALL carry `listingId`, `placementId`, `impressionId`, `modelVersion` and `position = index + 1` as `ListingGrid` passes them. `/search` SHALL announce its results through `ListingGrid` with `item_list_id = "search_results"` and `q` as the search term; `SearchImpressions` SHALL NOT be used. `RecommendationsRow` SHALL keep placement ids `home_feed` and `similar_items`. Existing `data-*` attributes and `data-testid` values SHALL be preserved.

#### Scenario: Card impression still fires once

- **WHEN** a `ListingGrid` with `placementId="home_feed"` and 3 visible cards (`impressionId` and `modelVersion` set per card) scrolls into view
- **THEN** exactly one `view_item_list` event is sent with 3 items in grid order, `index` 1, 2, 3, each carrying `itemId`, `impressionId` and `modelVersion`, all with `item_list_id="home_feed"` and one shared event group id

#### Scenario: Card click still records select_item

- **WHEN** a visitor clicks the image or the title of a card
- **THEN** a `select_item` event with the same attribution fields is sent before navigation to `/listing/<id>`, and navigation is not blocked

#### Scenario: Search results still emit a batched impression

- **WHEN** `/search?q=ao` renders 24 results
- **THEN** one `view_item_list` event with `item_list_id="search_results"`, `search_term="ao"` and 24 items indexed 1..24 in grid order is sent, and no `SearchImpressions` component is rendered

#### Scenario: Attribution survives the Suspense rework

- **WHEN** the recommendations row streams in after the hero
- **THEN** its list impression carries `item_list_id="home_feed"` and its card click events carry `placementId="home_feed"`
