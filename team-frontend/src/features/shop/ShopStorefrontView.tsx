import { Tabs } from "@/components/ui/Tabs";
import { ChatWithSellerButton } from "@/features/chat/ChatWithSellerButton";
import { FollowSellerButton } from "@/features/engagement/FollowSellerButton";
import { ListingGrid } from "@/features/listing/ListingGrid";
import type { StorefrontSort } from "@/features/listing/pdp";
import type { ViewListing, ViewStorefront } from "@/lib/gateway/listings";
import type { ViewShopRatingSummary } from "@/lib/gateway/reviews";
import { ShopHeaderCard } from "./ShopHeaderCard";

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
}: {
  sellerId: string;
  listings: ViewListing[];
  loggedIn: boolean;
  initialFollowing?: boolean;
  storefront?: ViewStorefront | null;
  summary: ViewShopRatingSummary;
  sort?: StorefrontSort;
}) {
  const sortedListings = sortListings(listings, sort);

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

      <div className="space-y-4">
        <h2 className="text-sm font-bold uppercase tracking-wider text-text-primary">
          SẢN PHẨM CỦA SHOP ({sortedListings.length})
        </h2>
        <ListingGrid
          listings={sortedListings}
          empty="Shop này hiện chưa có sản phẩm nào đang bày bán."
        />
      </div>
    </div>
  );
}
