## Decisions

- Cursor: opaque base64url of `(created_at, id)` of the last returned order; the page query is
  `seller_id = $principal AND (created_at, id) < ($c, $id) ORDER BY created_at DESC, id DESC LIMIT n+1`. An undecodable
  cursor is INVALID_ARGUMENT. A cursor minted for another filter or seller only gives a position, so rows stay the caller's.
- Search: `query` trimmed; blank is ignored. Matches `id ILIKE <escaped>%` OR `EXISTS (SELECT 1 FROM order_items WHERE
  order_id = orders.id AND title ILIKE %<escaped>%)`. Escape `\`, `%`, `_` with `ESCAPE '\'`. Scope is already narrowed by
  seller_id, so no trigram index is added (5,000 rows per seller).
- Filter precedence: non-empty `statuses` wins over `status_filter`; `UNSPECIFIED` inside `statuses` is INVALID_ARGUMENT;
  `status_filter = 0` means all.
- `status_counts`: one grouped query `SELECT status, count(*)` with `seller_id` and `query` only; zero-count statuses
  (PENDING..CANCELLED, five entries) are included in enum order so the UI needs no defaults. `page.total` is the count of
  the filtered set (statuses + query).
- Unset `page` (field absent): returns the full filtered list ordered `created_at DESC` as today, with empty `next_cursor`,
  `total` = list length, counts filled. Set `page` (even with empty cursor and size 0) selects paged mode with size 20.
- Items are loaded for the page orders only (one batched query by order id set instead of per order) to hold the 150 ms budget.
