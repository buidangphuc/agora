import { Card } from "@/components/ui/Card";
import { Image } from "@/components/ui/Image";
import { PriceTag } from "@/components/ui/PriceTag";
import { Rate } from "@/components/ui/Rate";
import { Tag } from "@/components/ui/Tag";
import { FavoriteButton } from "@/features/engagement/FavoriteButton";
import { TrackImpression } from "@/features/tracking/TrackImpression";
import { TrackLink } from "@/features/tracking/TrackLink";
import type { ViewListing } from "@/lib/gateway/listings";
import { getImageUrl } from "@/lib/media";
import { EAGER_IMAGE_COUNT } from "./gridClass";

/** Real review aggregate of a listing, when the gateway provides one. */
export interface ListingReviewSummary {
  average: number;
  count: number;
}

/** Same 1:1 box as Image, for a listing without a picture. */
function NoImage() {
  return (
    <div className="flex aspect-square w-full items-center justify-center bg-surface-page text-text-disabled">
      <svg
        viewBox="0 0 24 24"
        className="h-8 w-8"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
        role="img"
        aria-label="Không có ảnh"
      >
        <rect x="3" y="4" width="18" height="16" rx="2" />
        <circle cx="9" cy="10" r="1.5" />
        <path d="M21 16l-5-5-8 8" />
      </svg>
    </div>
  );
}

/**
 * The single product tile of the discovery routes. Only real data is shown:
 * the real price, a rating only for a review aggregate with count > 0, and a
 * Freeship tag only when the caller knows the listing ships free. The tracking
 * tree (TrackImpression > TrackLink on image and title) is unchanged.
 */
export function ListingCard({
  listing,
  placementId,
  impressionId,
  modelVersion,
  position,
  review,
  freeShip = false,
}: {
  listing: ViewListing;
  placementId?: string;
  impressionId?: string;
  modelVersion?: string;
  position?: number;
  /** Real review aggregate; omitted or count 0 renders no rating. */
  review?: ListingReviewSummary;
  /** True only when the listing actually ships free. */
  freeShip?: boolean;
}) {
  const imageSrc =
    listing.imageKeys && listing.imageKeys.length > 0
      ? getImageUrl(listing.imageKeys[0])
      : listing.imageUrl;
  const eager = position !== undefined && position <= EAGER_IMAGE_COUNT;

  return (
    <TrackImpression
      listingId={listing.id}
      placementId={placementId}
      impressionId={impressionId}
      modelVersion={modelVersion}
      position={position}
    >
      <Card hoverable className="group relative flex h-full flex-col">
        <div className="relative">
          <TrackLink
            listingId={listing.id}
            placementId={placementId}
            impressionId={impressionId}
            modelVersion={modelVersion}
            position={position}
            href={`/listing/${listing.id}`}
            className="block w-full"
          >
            {imageSrc ? (
              <Image
                src={imageSrc}
                alt={listing.title}
                aspect="square"
                loading={eager ? "eager" : "lazy"}
                className="rounded-b-none"
              />
            ) : (
              <NoImage />
            )}
          </TrackLink>

          {/* Favorite heart button */}
          <div className="absolute bottom-2 right-2 z-10">
            <FavoriteButton id={listing.id} initial={false} />
          </div>
        </div>

        <div className="flex flex-1 flex-col gap-2 p-3">
          <h3 className="line-clamp-2 min-h-10 text-sm font-normal leading-5 text-text-primary transition duration-150 group-hover:text-action-primary">
            <TrackLink listingId={listing.id} href={`/listing/${listing.id}`}>
              {listing.title}
            </TrackLink>
          </h3>

          {/* Reserved meta row: the card keeps one height with or without data. */}
          <div className="flex min-h-5 flex-wrap items-center gap-2 text-xs text-text-secondary">
            {freeShip && <Tag color="success">Freeship</Tag>}
            {review !== undefined && review.count > 0 && (
              <span className="inline-flex items-center gap-1">
                <Rate readOnly size="sm" value={review.average} />
                <span>({review.count})</span>
              </span>
            )}
          </div>

          <PriceTag price={listing.price} size="md" className="mt-auto" />
        </div>
      </Card>
    </TrackImpression>
  );
}
