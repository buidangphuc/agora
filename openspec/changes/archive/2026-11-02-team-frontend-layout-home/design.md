## Decisions

- Section headings become a plain `h2` with a description line (the fe-platform `Page` header style) instead of a bordered card bar; the `h2` ids (`home-feed-title`, `recently-viewed-title`) and text stay, so `aria-labelledby` and text queries still work.
- Links that act as controls (`Thử lại`, `Xem thêm gợi ý`, `Xem tất cả`, `Mua ngay`) use the `Link` facade button variants, which carry `min-h-touch`; no new facade props are needed.
- Hub icon boxes keep one shared class (the neutral-tile test) and move to a `primary-50` tint, a tier-1 token, not a raw palette colour.
- The flash-sale strip bleeds into the section padding on mobile (`-mx-4 px-4`) so cards scroll edge to edge inside the card; the page itself does not scroll sideways.
- The page column switches from `space-y` to `flex gap`, so blocks that render `null` add no stray spacing.

## Risks

- `RecommendationsRow`, `CategoryBar` and `ListingCard` keep their current look until their own pages are rebuilt, so the home page mixes old and new headings for a while.
- Taller hero and hub tiles push the feed lower on small phones.
