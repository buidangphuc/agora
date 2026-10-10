## ADDED Requirements

### Requirement: ListReviews serves the requested page in a stable order

`ListReviews` SHALL honour `PageRequest`: `cursor` is the offset of the first review to return (empty = 0) and
`page_size` defaults to 20 and is clamped to 100. Reviews SHALL be ordered newest first with the review id as
tiebreak. The response SHALL carry `total` (the count for the active rating filter) and a `next_cursor` that is
empty on the last page. A malformed cursor SHALL be rejected with `InvalidArgument`.

#### Scenario: Pages are disjoint and newest first

- **WHEN** a listing has 105 reviews and a client requests pages of 10 following `next_cursor` from the start
- **THEN** every review appears exactly once, in newest-first order, `total` is 105 on every page and the
  eleventh page holds the 5 oldest reviews with an empty `next_cursor`

#### Scenario: Page size is bounded

- **WHEN** a client requests `page_size` 500 for that listing
- **THEN** at most 100 reviews are returned and `next_cursor` is set

#### Scenario: A malformed cursor is rejected

- **WHEN** a client sends `cursor` "abc" or "-5"
- **THEN** the call fails with `InvalidArgument`

### Requirement: The product page requests the review page from the server

The PDP reviews list SHALL request only the server page for `?rpage=N` (with the `?rating=` filter) and derive
its page count from the server `total`, so listings with more than 100 reviews are fully reachable. A page
number beyond the last page SHALL show the last page.

#### Scenario: A listing with 105 reviews shows 11 pages

- **WHEN** a buyer opens a listing with 105 reviews
- **THEN** the reviews list shows 10 reviews and the `Pagination` offers 11 pages

#### Scenario: The last page shows the oldest reviews

- **WHEN** the buyer opens `?rpage=11` of that listing
- **THEN** 5 reviews are listed, they are the 5 oldest, and the `Pagination` marks page 11 as current
