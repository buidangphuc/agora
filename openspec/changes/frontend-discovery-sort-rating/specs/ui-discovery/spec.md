## MODIFIED Requirements

### Requirement: Discovery routes show only real data

Discovery routes SHALL display only data returned by the gateway. They SHALL NOT render fabricated ratings,
sold counts, strike-through prices computed from the price, discount badges, flash-sale countdowns or "% sold"
bars, Mall badges or rows derived from title or price, or hard-coded hero/hub figures. A block with no real
data SHALL be hidden; `Empty` is used only where the block is the page's purpose (search results, vouchers).
Features without a backend SHALL be hidden, not simulated: "Lưu mã" on `/vouchers`; no `localStorage` voucher wallet
SHALL exist. The "Bán chạy" sort and the rating filter on `/search` are backed by indexed `sold_count` and `rating_avg`
and are offered.

#### Scenario: No fabricated markers on the home page

- **WHEN** `/` is rendered with listings that have no review, discount or campaign data
- **THEN** the document contains no strike-through price, no "-NN%" badge, no "ĐÃ BÁN" text, no countdown, no
  MALL badge and no star rating

#### Scenario: Mall is never guessed

- **WHEN** a listing costs more than 5,000,000 or its title contains a brand keyword
- **THEN** its card shows no MALL badge

#### Scenario: Unsupported features are hidden

- **WHEN** `/search` and `/vouchers` are rendered
- **THEN** the sort options include "Bán chạy" and no voucher card has a "Lưu mã" button

### Requirement: Search page follows the Search List anatomy with URL state

`/search` SHALL render: a header with `Breadcrumb`, `h1` and the result count; a filter column with
`SavedSearches` and `FilterSidebar`; and a results column with `SortBar`, active-filter `Tag`s,
`ListingGrid` (`data-testid="search-results"`) and `Pagination`. All state SHALL live in the URL:
`q, category, seller, minRating, minPrice, maxPrice, sort, page`. Changing any filter or sort SHALL reset `page`.
Facet counts SHALL be shown beside each bucket. The page SHALL work for sort and filter links without
client JavaScript.

#### Scenario: Filters are shareable URLs

- **WHEN** a buyer selects the "100000-500000" price bucket and sort "price_asc"
- **THEN** the URL contains `minPrice=100000&maxPrice=500000&sort=price_asc`, and opening that URL in a new
  tab shows the same results and the same selected state

#### Scenario: Changing a filter resets the page

- **WHEN** a buyer on `/search?q=ao&page=3` selects a rating filter
- **THEN** the new URL has no `page` (or `page=1`) and shows the first page of the filtered results

#### Scenario: Active filters can be removed one by one

- **WHEN** `/search?q=ao&category=c1&minRating=4` is rendered
- **THEN** three closable `Tag`s are shown, and activating the category tag navigates to
  `/search?q=ao&minRating=4`

#### Scenario: Clear all filters

- **WHEN** at least one filter is active and the buyer activates "Xóa tất cả bộ lọc"
- **THEN** the URL becomes `/search` (keeping `q` only if present) and all tags disappear

#### Scenario: Sort options match what the server honours

- **WHEN** the `SortBar` is rendered
- **THEN** it offers Liên quan, Mới nhất, Bán chạy, Giá thấp đến cao and Giá cao đến thấp, selecting "Bán chạy" navigates to a URL
  with `sort=best_selling`, and the active link has `aria-current="page"`

#### Scenario: Best-selling sort orders results by sold count

- **GIVEN** listings with `sold_count` 7, 3 and 0 match the keyword
- **WHEN** a buyer selects "Bán chạy"
- **THEN** the results appear in the order 7, 3, 0 and opening `/search?q=<keyword>&sort=best_selling` in a new tab shows the same order

#### Scenario: An unknown sort value falls back to relevance

- **WHEN** a buyer opens `/search?q=<keyword>&sort=top_rated`
- **THEN** the page renders the relevance order with "Liên quan" marked `aria-current="page"` and no error
