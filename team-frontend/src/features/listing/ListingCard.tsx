import { formatPrice, formatSoldCount } from "@/components/ui/format";
import { FavoriteButton } from "@/features/engagement/FavoriteButton";
import { TrackLink } from "@/features/tracking/TrackLink";
import type { ViewListing } from "@/lib/gateway/listings";
import { getImageUrl } from "@/lib/media";

export function ListingCard({
  listing,
}: {
  listing: ViewListing;
}) {
  const imageSrc =
    listing.imageKeys && listing.imageKeys.length > 0
      ? getImageUrl(listing.imageKeys[0])
      : listing.imageUrl;

  const isMall =
    listing.price > 5000000 ||
    listing.title.toLowerCase().includes("chính hãng") ||
    listing.title.toLowerCase().includes("apple") ||
    listing.title.toLowerCase().includes("sony") ||
    listing.title.toLowerCase().includes("philips") ||
    listing.title.toLowerCase().includes("nike");

  // Discount percentage calculation
  const originalPrice = Math.round(listing.price * 1.25);
  const soldCount = listing.stock > 0 ? 120 + ((listing.stock * 3) % 850) : 85;

  return (
    <article className="group relative flex flex-col overflow-hidden rounded-xs border border-gray-200/80 bg-white shadow-shopee transition-all duration-200 hover:-translate-y-0.5 hover:border-brand hover:shadow-shopee-hover">
      {/* ── 1:1 Aspect Ratio Image & Official Badges ── */}
      <div className="relative aspect-square w-full overflow-hidden bg-gray-100">
        <TrackLink
          listingId={listing.id}
          href={`/listing/${listing.id}`}
          className="block h-full w-full"
        >
          {imageSrc ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={imageSrc}
              alt={listing.title}
              className="h-full w-full object-cover transition duration-300 group-hover:scale-105"
              loading="lazy"
            />
          ) : (
            <div className="grid h-full w-full place-items-center text-gray-300 bg-gray-50">
              <span className="text-3xl">🛍️</span>
            </div>
          )}
        </TrackLink>

        {/* Mall / Favorite Badge */}
        <div className="absolute top-0 left-0 z-10">
          {isMall ? (
            <span className="rounded-br-xs bg-[#d0011b] px-1.5 py-0.5 text-[9px] font-bold text-white uppercase tracking-wider shadow-2xs">
              Mall
            </span>
          ) : (
            <span className="rounded-br-xs bg-brand px-1.5 py-0.5 text-[9px] font-semibold text-white uppercase tracking-wider shadow-2xs">
              Yêu thích+
            </span>
          )}
        </div>

        {/* Discount Badge */}
        <div className="absolute top-0 right-0 z-10 flex flex-col items-center bg-[#ffe97a]/95 px-1 py-0.5 text-center text-[#ee4d2d] font-bold shadow-2xs">
          <span className="text-[10px] leading-tight font-extrabold">-20%</span>
          <span className="text-[8px] font-black uppercase text-white bg-[#ee4d2d] px-0.5 rounded-2xs mt-0.5">
            GIẢM
          </span>
        </div>

        {/* Favorite heart button */}
        <div className="absolute bottom-2 right-2 z-10">
          <FavoriteButton id={listing.id} initial={false} />
        </div>
      </div>

      {/* ── Card Content ── */}
      <div className="flex flex-1 flex-col p-2">
        {/* Title */}
        <h3 className="line-clamp-2 text-[12px] leading-4 text-[#222222] font-normal group-hover:text-brand transition min-h-[32px]">
          <TrackLink listingId={listing.id} href={`/listing/${listing.id}`}>
            {isMall && (
              <span className="mr-1 inline-block rounded-2xs bg-[#d0011b] px-1 py-0.2 text-[9px] font-bold text-white uppercase leading-none">
                Mall
              </span>
            )}
            {listing.title}
          </TrackLink>
        </h3>

        {/* Shopee Promo Tags */}
        <div className="mt-1.5 flex flex-wrap gap-1">
          <span className="rounded-2xs border border-red-400 bg-red-50/80 px-1 py-0.2 text-[9px] font-medium text-red-600">
            Giảm ₫50k
          </span>
          <span className="rounded-2xs bg-emerald-50 px-1 py-0.2 text-[9px] font-medium text-emerald-700">
            Freeship Xtra
          </span>
        </div>

        {/* Price & Strikethrough */}
        <div className="mt-2 flex items-baseline gap-1.5">
          <span className="text-[11px] text-gray-400 line-through">
            {formatPrice(originalPrice, listing.currency)}
          </span>
          <span className="text-[15px] font-semibold text-brand">
            {formatPrice(listing.price, listing.currency)}
          </span>
        </div>

        {/* Rating & Sold count */}
        <div className="mt-auto flex items-center justify-between pt-2 text-[11px] text-gray-500 border-t border-gray-100">
          <div className="flex items-center gap-1">
            <span className="text-yellow-400 text-xs">★</span>
            <span className="text-gray-700 font-medium">5.0</span>
          </div>
          <span className="text-gray-500 font-normal">
            {formatSoldCount(soldCount)}
          </span>
        </div>

        {/* Location */}
        <div className="mt-1 text-right text-[10px] text-gray-400 font-normal">
          TP. Hồ Chí Minh
        </div>
      </div>
    </article>
  );
}
