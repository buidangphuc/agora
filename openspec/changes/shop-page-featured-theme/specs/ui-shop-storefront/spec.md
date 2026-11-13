## ADDED Requirements

### Requirement: Featured listings on the shop page

`/shop/[id]` SHALL show a "Sản phẩm nổi bật" block above the product grid, listing the storefront's
`featured_listing_ids` in stored order, at most 6, each rendered as a listing card. An id SHALL be skipped silently
when its listing is not found, not published, or not owned by this shop. The block SHALL be absent when no item
remains. Featured listings SHALL NOT be repeated in the "Sản phẩm của shop" grid.

#### Scenario: Featured listings appear first in the chosen order

- **WHEN** a visitor opens `/shop/{id}` of a shop whose storefront features published listings B, A, C (in that order)
- **THEN** the block `featured-listings` contains exactly the cards B, A, C in that order, and the main grid
  contains none of them

- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/shop_storefront_ui.feature` `Featured listings appear first in the chosen order`

#### Scenario: A listing unpublished after being featured is skipped

- **WHEN** a visitor opens `/shop/{id}` and one of the 3 featured listings has been unpublished since it was saved
- **THEN** the page responds 200 with the 2 remaining featured cards, no error text, and the unpublished listing
  appears nowhere on the page

- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/shop_storefront_ui.feature` `A listing unpublished after being featured is skipped`

#### Scenario: A featured id of another shop is not shown

- **WHEN** the stored storefront lists a published listing owned by a different seller
- **THEN** THE SYSTEM SHALL NOT render that listing in the `featured-listings` block of this shop

- **VERIFIED BY**: `team-frontend/src/features/shop/featured.test.ts` `drops a listing owned by another seller`

#### Scenario: No featured listings means no block

- **WHEN** a visitor opens `/shop/{id}` of a shop with no featured ids, or whose featured ids are all unavailable
- **THEN** the page has no `featured-listings` element and the main grid shows all the shop's published listings

- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/shop_storefront_ui.feature` `A shop without featured listings has no featured block`

#### Scenario: At most 6 featured listings are shown

- **WHEN** the stored storefront holds 8 valid featured ids
- **THEN** the `featured-listings` block shows exactly the first 6, and duplicated ids show once

- **VERIFIED BY**: `team-frontend/src/features/shop/featured.test.ts` `caps at 6 and dedupes`

#### Scenario: A storefront outage does not break the page

- **WHEN** `GetStorefront` fails for the shop
- **THEN** THE SYSTEM SHALL still render the page with the product grid, no featured block and the default theme

- **VERIFIED BY**: `team-frontend/src/features/shop/ShopStorefrontView.test.tsx` `renders without a storefront`

### Requirement: The shop page applies the storefront theme

The hero header of `/shop/[id]` SHALL apply the storefront `theme`: `default`, `dark` or `festive` each select a
header colour and an accent colour; an empty or unknown key SHALL render as `default`. The theme SHALL change
nothing else on the page, and the hero card SHALL expose the resolved key as `data-theme`.

#### Scenario: A themed shop renders its theme

- **WHEN** a visitor opens `/shop/{id}` of a shop whose storefront theme is `festive`
- **THEN** the `shop-header-card` has `data-theme="festive"` and its computed header background colour differs from
  the one of a `default` shop, while the product grid markup is unchanged

- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/shop_storefront_ui.feature` `A themed shop renders its theme`

#### Scenario: An unknown theme key falls back to default

- **WHEN** the stored theme key is empty or not one of `default`, `dark`, `festive`
- **THEN** THE SYSTEM SHALL render `data-theme="default"` and never place the raw key in a class or style

- **VERIFIED BY**: `team-frontend/src/features/shop/shopTheme.test.ts` `unknown key resolves to default`

#### Scenario: Theme text stays readable

- **WHEN** any of the three themes is rendered
- **THEN** header text on header background has contrast of at least 4.5:1

- **VERIFIED BY**: `team-frontend/src/features/shop/shopTheme.test.ts` `every theme meets AA contrast`
