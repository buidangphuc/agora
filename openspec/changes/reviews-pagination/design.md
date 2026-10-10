## Decisions

- **Cursor = decimal offset string.** The PDP needs random access to page N (`?rpage=N`), which a
  keyset cursor cannot give. `team-search` already uses an offset cursor behind the opaque `cursor` field, so
  this follows platform convention. The frontend computes `cursor = (rpage-1) * 10`.
- **Stable order.** `ORDER BY created_at DESC, id DESC` in Postgres; the in-memory repository sorts the same
  way. Existing index `(listing_id, created_at DESC)` serves the scan.
- **Bounds.** `page_size` 0 -> 20; above 100 -> 100 (was: reset to 20). Negative/non-numeric cursor ->
  `InvalidArgument`. An offset past the end returns an empty page with the real `total` and empty `next_cursor`.
- **Total.** `PageResponse.total` is the filtered count (rating filter applied), so the UI derives pages as
  `ceil(total/10)`. A page request beyond the last page makes the frontend re-request the last page.
- **Frontend.** New `listReviewsPage(listingId, {rating, page})` returns `{reviews, total}`. `listReviews`
  (first 100, unfiltered) stays for the AI summary, which has its own promise so the list and the summary no
  longer share one fetch.
