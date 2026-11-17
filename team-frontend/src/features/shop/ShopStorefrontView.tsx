import { Link } from "@/components/ui/Link";
import { Tabs } from "@/components/ui/Tabs";
import { ChatWithSellerButton } from "@/features/chat/ChatWithSellerButton";
import { FollowSellerButton } from "@/features/engagement/FollowSellerButton";
import { ListingGrid } from "@/features/listing/ListingGrid";
import type { StorefrontSort } from "@/features/listing/pdp";
import type { ViewListing, ViewStorefront } from "@/lib/gateway/listings";
import type { ViewShopRatingSummary } from "@/lib/gateway/reviews";
import { ShopHeaderCard } from "./ShopHeaderCard";
import { resolveFeatured } from "./featured";

/** Listings in the order of the `?sort=` param (default: as returned). */
export function sortListings(
  listings: ViewListing[],
  sort: StorefrontSort,
): ViewListing[] {
  if (sort === "price_asc")
    return [...listings].sort((a, b) => a.price - b.price);
  if (sort === "price_desc")
    return [...listings].sort((a, b) => b.price - a.price);
  return listings;
}

function sortHref(sellerId: string, sort: string): string {
  return sort === "all"
    ? `/shop/${sellerId}`
    : `/shop/${sellerId}?sort=${sort}`;
}

/**
 * Shop storefront: the shared `ShopHeaderCard` (hero), the price-sort tabs as
 * links driven by `?sort=`, and the product grid. A server component: only the
 * follow and chat buttons are client leaves.
 */
export function ShopStorefrontView({
  sellerId,
  listings,
  loggedIn,
  initialFollowing = false,
  storefront = null,
  summary,
  sort = "all",
  fetchedFeatured = [],
}: {
  sellerId: string;
  listings: ViewListing[];
  loggedIn: boolean;
  initialFollowing?: boolean;
  storefront?: ViewStorefront | null;
  summary: ViewShopRatingSummary;
  sort?: StorefrontSort;
  fetchedFeatured?: (ViewListing | null)[];
}) {
  const featured = resolveFeatured(storefront, listings, fetchedFeatured);
  const featuredIds = new Set(featured.map((l) => l.id));
  const sortedListings = sortListings(
    listings.filter((l) => !featuredIds.has(l.id)),
    sort,
  );

  return (
    <div className="space-y-6">
      <ShopHeaderCard
        variant="hero"
        sellerId={sellerId}
        storefront={storefront}
        summary={summary}
        productCount={listings.length}
        actions={
          <>
            <FollowSellerButton
              sellerId={sellerId}
              initialFollowing={initialFollowing}
            />
            <ChatWithSellerButton
              sellerId={sellerId}
              listingId={listings[0]?.id ?? ""}
              loggedIn={loggedIn}
            />
          </>
        }
      />

      <Tabs
        variant="pills"
        activeId={sort}
        hrefFor={(id) => sortHref(sellerId, id)}
        items={[
          { id: "all", label: `Tất cả sản phẩm (${listings.length})` },
          { id: "price_asc", label: "Giá: Thấp đến Cao" },
          { id: "price_desc", label: "Giá: Cao đến Thấp" },
        ]}
      />

      {featured.length > 0 && (
        <div data-testid="featured-listings" className="space-y-4">
          <h2 className="text-sm font-bold uppercase tracking-wider text-text-primary">
            SẢN PHẨM NỔI BẬT
          </h2>
          <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3">
            {featured.map((l) => (
              <li key={l.id}>
                <Link href={`/listing/${l.id}`} variant="outline" size="xs">
                  {l.title}
                </Link>
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="space-y-4">
        <h2 className="text-sm font-bold uppercase tracking-wider text-text-primary">
          SẢN PHẨM CỦA SHOP ({sortedListings.length})
        </h2>
        <ListingGrid
          listings={sortedListings}
          placementId="shop_storefront"
          empty="Shop này hiện chưa có sản phẩm nào đang bày bán."
        />
      </div>
    </div>
  );
}
