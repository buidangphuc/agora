## Why

Story 1 (sort by "Bán chạy") and story 3 (rating filter) of initiative `discovery-seo` need the buyer app to expose
the controls that `search-bestseller-rating` makes real in team-search. `SortBar` hides "Bán chạy" and the filter
sidebar offers no rating group. This slice owns the team-frontend half only; it was split out of
`search-bestseller-rating` so that each change touches one owner repo.

initiative: discovery-seo

## What changes

- **`SortBar` / `SortSelect`**: a "Bán chạy" option; URL `sort=best_selling`; the data layer maps it to
  `SORT_BY_BEST_SELLING`.
- **`FilterSidebar` and the mobile drawer**: a rating group (`data-testid="facet-ratings"`) with "Từ 4 sao" and
  "Từ 3 sao" and their `facets.ratings` counts; URL `minRating=4|3`, sent as `min_rating`; selected state; a closable
  chip counted as one active filter. Any other `minRating` is ignored. `src/features/search/url.ts` owns the state.
- **Old links**: a `rating=` parameter is still ignored and renders a normal results page (no error, no chip).
- **Deltas**: `frontend-search-rating` (the "no rating filter" requirement is replaced; the old-link requirement is
  kept) and `ui-discovery` (real-data rule, URL state, sort options).
- **Rewritten, not deleted** (tasks 2.x): FEATURES entries `discovery.sort-options-match-what-the-server-honours`,
  `discovery.search-rating-the-search-page-shows-no-rating-filter`,
  `discovery.search-rating-an-old-rating-link-renders-the-results-without-error`; e2e
  `frontend/search_rating_removed.feature` and `frontend/search_facets.feature` (line 39).
- Depends on `search-bestseller-rating` for the backend: `SORT_BY_BEST_SELLING`, `min_rating` accepted with floors 3
  and 4, and `facets.ratings` that ignores its own `min_rating` (C7) while other filters still apply. Uses only
  `SearchService.SearchListings`; no new RPC.
- Shared hot files with `search-attribution` and `frontend-seo`: `src/features/search/SearchBlocks.tsx` and
  `src/app/(shop)/search/page.tsx`. Keep edits minimal (state pass-through) and rebase on whichever lands first.

## NFR

- No extra gateway call: sort and rating ride the existing `SearchListings` request.
- Sort and filter links work without client JavaScript (existing rule).
- Page render with the new controls adds no more than 10% to `/search` server render time against the local baseline: measured, non-gating (C9).

## Data classification

No personal or sensitive data. `sort` and `minRating` are non-identifying URL parameters; they are not logged with
user ids and no new cookie, storage or beacon field is added. Consent basis and retention: not applicable.

## Threat model

- Assets: result integrity and page availability.
- Abuse cases: a crafted `minRating` (`9`, `-1`, `abc`, repeated) or `sort` value; an old `rating=` link. Mitigation:
  the frontend only accepts 3 or 4 and the known sort keys, otherwise ignores them, so the gateway never receives an
  out-of-range `min_rating` and the page never errors.
- Authz: `/search` is public; no permission rule applies.
