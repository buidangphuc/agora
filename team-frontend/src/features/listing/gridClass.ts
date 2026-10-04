/**
 * The one product-grid layout: 2 columns at 375px, 3 at sm, 4 at md, 6 at lg.
 * ListingGrid and ListingGridSkeleton share it so a streamed result never
 * shifts the layout (CLS = 0).
 */
export const LISTING_GRID_CLASS =
  "grid grid-cols-2 gap-2 sm:grid-cols-3 sm:gap-3 md:grid-cols-4 lg:grid-cols-6";

/** Cards 1-6 of a page are above the fold: their image loads eagerly. */
export const EAGER_IMAGE_COUNT = 6;
