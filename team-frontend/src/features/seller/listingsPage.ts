import {
  type ListingPage,
  type ViewListing,
  listMyListings,
} from "@/lib/gateway/listings";

export const LISTINGS_PAGE_SIZE = 20;

/**
 * Page `page` (1-based) of the seller's listings. The gateway is cursor-based,
 * so page N is reached by walking N-1 cursors; walking past the last page
 * yields an empty page that still carries the real `total`.
 */
export async function getListingsPage(
  page: number,
  pageSize = LISTINGS_PAGE_SIZE,
): Promise<ListingPage> {
  let cursor = "";
  for (let i = 1; i < page; i++) {
    const skipped = await listMyListings({ cursor, pageSize });
    if (skipped.nextCursor === "") {
      return { items: [], nextCursor: "", total: skipped.total };
    }
    cursor = skipped.nextCursor;
  }
  return listMyListings({ cursor, pageSize });
}

/**
 * Every listing of the seller (for pickers such as ads and bundles), walking
 * the cursor up to `maxPages` pages so a large catalogue is still selectable.
 */
export async function getAllListings(
  maxPages = 10,
  pageSize = LISTINGS_PAGE_SIZE,
): Promise<ViewListing[]> {
  const all: ViewListing[] = [];
  let cursor = "";
  for (let i = 0; i < maxPages; i++) {
    const page = await listMyListings({ cursor, pageSize });
    all.push(...page.items);
    if (page.nextCursor === "") break;
    cursor = page.nextCursor;
  }
  return all;
}
