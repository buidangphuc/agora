import { Badge } from "@/components/ui/Badge";
import { PriceTag } from "@/components/ui/PriceTag";
import { formatSoldCount } from "@/components/ui/format";
import { FavoriteButton } from "@/features/engagement/FavoriteButton";
import { TrackImpression } from "@/features/tracking/TrackImpression";
import { TrackLink } from "@/features/tracking/TrackLink";
import type { ViewListing } from "@/lib/gateway/listings";
import { getImageUrl } from "@/lib/media";

export function ListingCard({
  listing,
  placementId,
  impressionId,
  modelVersion,
  position,
}: {
  listing: ViewListing;
  placementId?: string;
  impressionId?: string;
  modelVersion?: string;
  position?: number;
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
    <TrackImpression
      listingId={listing.id}
      placementId={placementId}
      impressionId={impressionId}
      modelVersion={modelVersion}
      position={position}
    >
      <article className="group relative flex flex-col h-full overflow-hidden rounded-xl border border-gray-200/90 bg-white shadow-preline-card transition-all duration-200 hover:-translate-y-1 hover:border-primary-400 hover:shadow-preline-hover">
        {/* ── 1:1 Aspect Ratio Image & Official Badges ── */}
        <div className="relative aspect-square w-full overflow-hidden bg-gray-50">
          <TrackLink
            listingId={listing.id}
            placementId={placementId}
            impressionId={impressionId}
            modelVersion={modelVersion}
            position={position}
            href={`/listing/${listing.id}`}
            className="block h-full w-full"
          >
            {imageSrc ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={imageSrc}
                alt={listing.title}
                className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
                loading="lazy"
              />
            ) : (
              <div className="grid h-full w-full place-items-center text-gray-300 bg-gray-50">
                <span className="text-3xl">🛍️</span>
              </div>
            )}
          </TrackLink>

          {/* Mall / Yêu thích Badge */}
          <div className="absolute top-2 left-2 z-10">
            {isMall ? (
              <Badge variant="mall" size="xs">
                MALL
              </Badge>
            ) : (
              <Badge variant="primary" size="xs">
                Yêu thích+
              </Badge>
            )}
          </div>

          {/* Discount Badge */}
          <div className="absolute top-2 right-2 z-10">
            <Badge variant="discount" size="xs">
              -20%
            </Badge>
          </div>

          {/* Favorite heart button */}
          <div className="absolute bottom-2 right-2 z-10 transition-transform active:scale-90">
            <FavoriteButton id={listing.id} initial={false} />
          </div>
        </div>

        {/* ── Card Content ── */}
        <div className="flex flex-1 flex-col p-3">
          {/* Title */}
          <h3 className="line-clamp-2 text-xs font-normal text-gray-800 leading-snug group-hover:text-primary-600 transition min-h-[34px]">
            <TrackLink listingId={listing.id} href={`/listing/${listing.id}`}>
              {listing.title}
            </TrackLink>
          </h3>

          {/* E-commerce Promo Tags */}
          <div className="mt-2 flex flex-wrap gap-1">
            <Badge variant="danger" size="xs">
              Giảm ₫50k
            </Badge>
            <Badge variant="success" size="xs">
              Freeship
            </Badge>
          </div>

          {/* Price & Strikethrough using unified PriceTag */}
          <div className="mt-2.5">
            <PriceTag
              price={listing.price}
              originalPrice={originalPrice}
              size="md"
            />
          </div>

          {/* Rating, Sold count & Location */}
          <div className="mt-auto flex items-center justify-between pt-2.5 text-xs text-gray-500 border-t border-gray-100">
            <div className="flex items-center gap-1">
              <span className="text-amber-400 text-xs">★</span>
              <span className="text-gray-700 font-semibold text-[11px]">
                5.0
              </span>
            </div>
            <span className="text-gray-400 text-[11px]">
              {formatSoldCount(soldCount)}
            </span>
          </div>

          <div className="mt-1 text-right text-[11px] text-gray-400">
            TP. Hồ Chí Minh
          </div>
        </div>
      </article>
    </TrackImpression>
  );
}
