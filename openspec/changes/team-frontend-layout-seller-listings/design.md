## Decisions

- The listing form splits into two fieldsets (main sections and status) inside one grid, both `disabled={pending || readOnly}`, so the pending and read-only behaviour is unchanged while the status card can sit in a sticky right column from `lg`.
- The action bar stays a direct child of the form: inside the right column its `sticky bottom-0` would only stick within that column, so on mobile it would stop following the scroll. From `md` it is a static rounded bar under the grid.
- The product table is placed outside `CardContent` so rows span the card; the filter bar, empty/error states and pagination get their own `p-4 sm:p-6` bands. The pagination band uses `empty:hidden` because `SellerPagination` renders nothing for one page.
- The stock column hides below `sm` through the platform `hideBelow`; the stock is repeated in the product cell with `sm:hidden`, so a phone still sees it without a horizontal scroll of the table.
- The textarea moves to the platform input recipe values (rounded-xl, border-strong, px-4 py-3, text-base, min-h-touch) since there is no platform Textarea yet.

## Risks

- `ProductList` and `ProductTable` render inside the `/seller` dashboard, which the sibling change for the seller dashboard also restyles; the two changes touch different files but should be merged one after the other and the dashboard checked once both land.
- The visual check ran without the gateway using an unsigned session cookie: the list was verified in its error state and the form empty; populated rows, the image grid and variant rows were verified through unit tests only.
