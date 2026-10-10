## Purpose

Defines how the buyer search page in `team-frontend` treats rating while search indexes no rating: no rating control
is offered, and an old link that carries a rating still opens a working results page.

## ADDED Requirements

### Requirement: The search page offers no rating filter

The `/search` page SHALL NOT render a rating filter group, a rating active-filter chip or a rating option in the
mobile filter drawer, whatever the gateway returns in `facets.ratings`. Links the page builds (sort, facet,
pagination, clear-filters) and the filter form SHALL NOT carry a `rating` parameter, and a rating SHALL NOT count as
an active filter.

#### Scenario: The search page shows no rating filter

- **GIVEN** published listings across categories and price ranges are indexed
- **WHEN** a buyer opens the search results for those listings
- **THEN** the filter sidebar shows the category and price groups and no `facet-ratings` group or "Đánh giá" filter, and no sort or pagination link on the page contains `rating=`

### Requirement: An old rating link opens a normal results page

The frontend SHALL ignore a `rating` URL parameter on `/search`: it SHALL NOT send a minimum rating to the gateway,
SHALL render the same results as the same URL without `rating`, and SHALL NOT show an error state or a rating chip
for it.

#### Scenario: An old rating link renders the results without error

- **GIVEN** a published listing whose title carries a unique keyword is indexed
- **WHEN** a buyer opens `/search?q=<keyword>&rating=4`
- **THEN** the page lists that listing exactly as `/search?q=<keyword>` does, shows no error or empty state, shows no rating chip, and its sort links do not contain `rating=`
