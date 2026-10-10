## Why

`team-engagement`'s `ListReviews` ignores the `PageRequest` it is sent: it always serves the first page and
defaults to 20 when no size is given. `team-frontend` works around this by fetching `page.pageSize=100` and
paginating client-side, so a listing with more than 100 reviews silently loses the rest and the page count is
wrong.

## What Changes

- `team-engagement`: `ListReviews` honours `PageRequest` (`cursor` = integer offset, the same convention
  `team-search` uses; `page_size` default 20, clamped to 100), orders by `created_at DESC, id DESC` (stable
  tiebreak), and returns `PageResponse{next_cursor, total}` (`next_cursor` empty on the last page; `total` is
  the count for the active rating filter). A malformed cursor is `InvalidArgument`.
- `team-frontend` (reviews): the PDP asks the server for the page `?rpage=N` (and the `?rating=` filter) instead
  of fetching everything and slicing; page count comes from the server `total`.
- Repos: `team-engagement`, `team-frontend`, `platform-e2e`. Capability: new `review-pagination`.

## Non-goals

- No proto change (`PageRequest`/`PageResponse` already exist; no gateway change, `ListReviews` is forwarded as is).
- No keyset/seek cursors, no change to review creation, the rating summary or the AI review summary (it keeps
  its first-100 sample).
- No change to the 10-per-page size or the `Pagination` component.
