import Link from "next/link";

import { Avatar } from "@/components/ui/Avatar";
import { Empty } from "@/components/ui/Empty";
import { Image } from "@/components/ui/Image";
import { Pagination } from "@/components/ui/Pagination";
import { Rate } from "@/components/ui/Rate";
import { Tag } from "@/components/ui/Tag";
import { REVIEWS_PAGE_SIZE, pdpHref } from "@/features/listing/pdp";
import type { ViewReview, ViewReviewsPage } from "@/lib/gateway/reviews";
import { ReviewHelpfulButton } from "./ReviewHelpfulButton";
import { WriteReviewButton } from "./WriteReviewButton";

function ReviewItem({ review }: { review: ViewReview }) {
  return (
    <li data-testid="review-item" className="space-y-2 py-4">
      <div className="flex items-center gap-3">
        <Avatar name={review.userName} size="sm" />
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="text-sm font-semibold text-text-primary">
            {review.userName}
          </span>
          {review.verifiedPurchase && (
            <Tag color="success" data-testid="verified-purchase">
              ✓ Đã mua hàng
            </Tag>
          )}
          <span className="text-xs text-text-secondary">
            {review.createdAt}
          </span>
        </div>
      </div>

      <Rate readOnly size="sm" value={review.rating} />

      <p className="text-sm leading-relaxed text-text-primary">
        {review.comment}
      </p>

      {review.mediaUrls.length > 0 && (
        <div className="flex flex-wrap gap-2 pt-1">
          {review.mediaUrls.map((url) => (
            <div key={url} className="h-16 w-16 shrink-0">
              <Image src={url} alt="Ảnh đánh giá" aspect="square" />
            </div>
          ))}
        </div>
      )}

      <div className="pt-1">
        <ReviewHelpfulButton
          reviewId={review.id}
          listingId={review.listingId}
          initialCount={review.helpfulCount}
        />
      </div>
    </li>
  );
}

/**
 * The reviews of the "Đánh giá" section: filtered by `?rating=`, 10 per page
 * (`?rpage=`) as served by team-engagement (the page is requested, not sliced), with Empty states that give the buyer a way out. An async server
 * component, rendered inside its own Suspense boundary.
 */
export async function ReviewList({
  listingId,
  productTitle,
  pagePromise,
  rating,
  variant,
  loggedIn,
}: {
  listingId: string;
  productTitle?: string;
  pagePromise: Promise<ViewReviewsPage>;
  rating: number;
  variant?: string;
  loggedIn: boolean;
}) {
  const result = await pagePromise;

  if (result.total === 0 && rating === 0) {
    return (
      <Empty
        description="Chưa có đánh giá nào"
        action={
          loggedIn ? (
            <WriteReviewButton
              listingId={listingId}
              productTitle={productTitle}
            />
          ) : undefined
        }
      />
    );
  }

  if (result.total === 0) {
    return (
      <Empty
        description={`Không có đánh giá ${rating} sao`}
        action={
          <Link
            href={pdpHref(listingId, { variant }, "reviews")}
            replace
            scroll={false}
            className="inline-flex items-center justify-center rounded-lg border border-border-strong bg-surface-card px-4 py-2 text-sm font-medium text-text-primary shadow-sm transition duration-150 hover:bg-surface-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2"
          >
            Xem tất cả
          </Link>
        }
      />
    );
  }

  return (
    <div className="space-y-4">
      <ul className="divide-y divide-border-subtle">
        {result.reviews.map((r) => (
          <ReviewItem key={r.id} review={r} />
        ))}
      </ul>
      <Pagination
        current={result.page}
        total={result.total}
        pageSize={REVIEWS_PAGE_SIZE}
        hrefFor={(p) =>
          pdpHref(listingId, { variant, rating, rpage: p }, "reviews")
        }
      />
    </div>
  );
}
