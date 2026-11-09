# frontend-search-rating Specification

## Purpose
Defines how the buyer search page in `team-frontend` treats rating while search indexes no rating: no rating control
is offered, and an old link that carries a rating still opens a working results page.

## Requirements

### Requirement: An old rating link opens a normal results page

The frontend SHALL ignore a `rating` URL parameter on `/search`: it SHALL NOT send a minimum rating to the gateway
because of it, SHALL render the same results as the same URL without `rating`, and SHALL NOT show an error state or
a rating chip for it. Only `minRating` selects a rating filter.

#### Scenario: An old rating link renders the results without error

- **GIVEN** a published listing whose title carries a unique keyword is indexed
- **WHEN** a buyer opens `/search?q=<keyword>&rating=4`
- **THEN** the page lists that listing exactly as `/search?q=<keyword>` does, shows no error or empty state, shows no rating chip, and its sort links do not contain `rating=`

### Requirement: The search page offers a rating filter

The `/search` page SHALL render a rating group (`data-testid="facet-ratings"`) in the filter sidebar and in the mobile
filter drawer with the options "Từ 4 sao" and "Từ 3 sao", each with its `facets.ratings` count. Choosing one SHALL
navigate to a URL that carries `minRating=4` or `minRating=3`, reset `page`, and send that value as `min_rating` to
the gateway. The selected option SHALL be marked, and a closable chip SHALL show the active rating filter as one
active filter; removing the chip SHALL drop `minRating`. Sort, facet, pagination and clear-filters links SHALL keep
`minRating` and SHALL NOT carry a `rating` parameter. A `minRating` other than 3 or 4 (including a non-numeric value) SHALL be ignored. When `minRating` is repeated, the first value SHALL win, with no error. The rating SHALL count once in the active-filter count shown on the drawer trigger.
The counts SHALL be shown as the gateway returns them, and the rating group SHALL stay visible with all its options
while a rating is selected.

#### Scenario: The search page shows the rating filter

- **GIVEN** published listings, some with verified reviews, are indexed
- **WHEN** a buyer opens the search results for those listings
- **THEN** the filter sidebar shows a `facet-ratings` group with "Từ 4 sao" and "Từ 3 sao" and their counts, and no link on the page contains `rating=`

#### Scenario: Choosing a rating filters the results by URL

- **GIVEN** listings with `rating_avg` 4.5 and 2 match the keyword
- **WHEN** a buyer selects "Từ 4 sao"
- **THEN** the URL contains `minRating=4`, the results list only the 4.5 listing, "Từ 4 sao" is marked selected, and opening the same URL in a new tab shows the same results and selection

#### Scenario: The rating group keeps both options while one is selected

- **GIVEN** listings with `rating_avg` 4.5, 3.5 and 2 match the keyword
- **WHEN** a buyer opens `/search?q=<keyword>&minRating=4`
- **THEN** the `facet-ratings` group still shows both "Từ 4 sao" and "Từ 3 sao", with the "Từ 3 sao" count equal to 2 (the count ignores the selected rating)

#### Scenario: The rating chip removes the filter

- **WHEN** a buyer on `/search?q=<keyword>&minRating=4` removes the rating chip
- **THEN** the URL no longer carries `minRating`, the results include the listing with `rating_avg` 2, and the drawer trigger's active-filter count changes from 1 to 0

#### Scenario: The rating filter is offered in the mobile drawer

- **WHEN** a buyer opens the filter drawer on a 375 px wide viewport and selects "Từ 3 sao"
- **THEN** the URL contains `minRating=3` and the results list only listings with `rating_avg` of at least 3

#### Scenario: An unsupported minRating value is ignored

- **WHEN** a buyer opens `/search?q=<keyword>&minRating=9`, and separately `/search?q=<keyword>&minRating=abc`
- **THEN** each page lists the same results as `/search?q=<keyword>`, shows no error and no rating chip, and no gateway request carries `min_rating`

#### Scenario: A repeated minRating uses the first value

- **WHEN** a buyer opens `/search?q=<keyword>&minRating=4&minRating=3`
- **THEN** the page behaves as `minRating=4`: one rating chip for "Từ 4 sao", only listings with `rating_avg` of at least 4, and no error

#### Scenario: A rating filter with no match shows the empty state

- **GIVEN** no listing matching the keyword has `rating_avg` of at least 4
- **WHEN** a buyer opens `/search?q=<keyword>&minRating=4`
- **THEN** the page shows the search empty state with the rating chip and zero results, and not an error
