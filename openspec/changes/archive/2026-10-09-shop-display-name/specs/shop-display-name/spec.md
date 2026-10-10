## ADDED Requirements

### Requirement: Storefront carries a seller-owned display name

`listing.v1.Storefront` SHALL expose `display_name` (field 7). team-domain SHALL be its only owner: only the
owning seller can set it through `UpsertStorefront` (seller id forced from the authenticated principal), it
SHALL be trimmed and, when non-empty, 1 to 80 characters with no control characters, and `GetStorefront`
SHALL return it. An empty value means "unset". The contract change SHALL be additive, and `buf lint` and
`buf breaking` SHALL pass.

#### Scenario: The name is returned for a storefront

- **WHEN** seller S upserts a storefront with `display_name` "  Tiem Hoa Nho  " and a visitor calls
  `GetStorefront` for S
- **THEN** the response storefront has `display_name` equal to "Tiem Hoa Nho"

#### Scenario: A name change is reflected

- **WHEN** seller S upserts a storefront with `display_name` "Tiem Hoa Nho" and later upserts it again with
  "Tiem Hoa Lon"
- **THEN** the next `GetStorefront` and `BatchGetStorefronts` for S return "Tiem Hoa Lon", and the
  buyer's `/shop/<S>` page shows "Tiem Hoa Lon" without a cache purge

#### Scenario: Invalid names are rejected

- **WHEN** a seller upserts a storefront with a `display_name` longer than 80 characters or containing a
  control character
- **THEN** the call fails with `InvalidArgument` and the stored name is unchanged

#### Scenario: A seller cannot set another seller's name

- **WHEN** seller A sends an `UpsertStorefront` whose body names `seller_id` of seller B with a
  `display_name`
- **THEN** the name is stored on A's own storefront and B's name is unchanged

### Requirement: Batch lookup of shop display names

`ListingService.BatchGetStorefronts` SHALL accept up to 100 `seller_ids` and return one `ShopSummary`
(`seller_id`, `display_name`, `slug`) per seller that has a storefront, using a single store query. Unknown
sellers SHALL be omitted, duplicates ignored, and more than 100 ids SHALL fail with `InvalidArgument`. The
gateway SHALL only forward the call (no composition, no business logic).

#### Scenario: A cart with two sellers returns both names in one batch

- **WHEN** a buyer's cart holds items from sellers A ("Shop Alpha") and B ("Shop Beta") and the cart page
  loads
- **THEN** the frontend wrapper issues exactly one `BatchGetStorefronts` call with both seller ids, the
  response holds both names, and each cart group header shows its own name

#### Scenario: An unknown or deleted seller gives an empty name

- **WHEN** `BatchGetStorefronts` is called with seller ids A (has a name) and Z (no storefront row)
- **THEN** the response contains an entry for A only, the call succeeds, and the wrapper maps Z to an empty
  `displayName`

#### Scenario: Duplicates and oversize requests

- **WHEN** `BatchGetStorefronts` is called with the same seller id three times, or with 101 distinct ids
- **THEN** the first returns a single entry for that seller and the second fails with `InvalidArgument`

#### Scenario: Following list resolves names in one batch

- **WHEN** a buyer follows three sellers and opens `/account/following`
- **THEN** one `ListFollowedSellers` call and one `BatchGetStorefronts` call are made, and each row shows
  the seller's display name

### Requirement: The UI renders a deterministic shop label

team-frontend SHALL build every shop label through a single `shopLabel(sellerId, displayName)` helper: the
trimmed display name when non-empty, otherwise `Shop #<first 6 characters of sellerId>`. A failed or empty
name lookup SHALL NOT fail the page. The cart (`ViewCartItem.sellerDisplayName`), the PDP shop header,
`/shop/<id>`, `/account/following` and the seller pages SHALL use it.

#### Scenario: Fallback when the seller has no name

- **WHEN** the cart contains an item whose `sellerId` is "abc123xyz" and no name is available for it
- **THEN** the group header reads "Shop #abc123"

#### Scenario: Lookup failure does not break the page

- **WHEN** `BatchGetStorefronts` fails or returns `Unimplemented` while the cart loads
- **THEN** the cart still renders every item and every group header uses the "Shop #<6 chars>" fallback

#### Scenario: Product detail shop header shows the name

- **WHEN** a buyer opens a listing whose seller has display name "Tiem Hoa Nho"
- **THEN** the shop header shows "Tiem Hoa Nho" and links to `/shop/<sellerId>`
