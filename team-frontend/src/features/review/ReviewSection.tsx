import { Suspense } from "react";

import { Card } from "@/components/ui/Card";
import { Progress } from "@/components/ui/Progress";
import { Rate } from "@/components/ui/Rate";
import { Skeleton } from "@/components/ui/Skeleton";
import { starCount } from "@/features/listing/pdp";
import type {
  ViewRatingSummary,
  ViewReview,
  ViewReviewsPage,
} from "@/lib/gateway/reviews";
import { AiReviewSummary } from "./AiReviewSummary";
import { ReviewList } from "./ReviewList";
import { ReviewRatingFilter } from "./ReviewRatingFilter";
import { WriteReviewButton } from "./WriteReviewButton";

const STARS = [5, 4, 3, 2, 1];

/** Footprint of the AI summary card while team-ai answers. */
export function AiSummarySkeleton() {
  return (
    <Card className="p-5">
      <Skeleton variant="text" lines={4} />
    </Card>
  );
}

/** Footprint of the review list while it loads. */
export function ReviewListSkeleton() {
  return (
    <div className="space-y-6 py-4">
      <Skeleton variant="text" lines={3} />
      <Skeleton variant="text" lines={3} />
    </div>
  );
}

/**
 * "Đánh giá" section (`#reviews`, always rendered). The summary box, filter and
 * heading come from the rating summary (critical data); the AI summary and the
 * review list stream in their own Suspense boundaries.
 */
export function ReviewSection({
  listingId,
  productTitle,
  summary,
  reviewsPromise,
  pagePromise,
  rating,
  variant,
  loggedIn,
}: {
  listingId: string;
  productTitle?: string;
  summary: ViewRatingSummary;
  /** First-100 sample for the AI summary. */
  reviewsPromise: Promise<ViewReview[]>;
  /** The server page of the list for `?rpage=` / `?rating=`. */
  pagePromise: Promise<ViewReviewsPage>;
  rating: number;
  variant?: string;
  loggedIn: boolean;
}) {
  const rated = summary.reviewCount > 0;

  return (
    <section
      id="reviews"
      aria-labelledby="reviews-heading"
      className="scroll-mt-40"
    >
      <Card className="space-y-5 p-6">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border-subtle pb-4">
          <div>
            <h2
              id="reviews-heading"
              className="text-lg font-semibold text-text-primary"
            >
              ĐÁNH GIÁ SẢN PHẨM
            </h2>
            <p className="mt-0.5 text-sm text-text-secondary">
              Nhận xét thực tế từ người mua hàng đã trải nghiệm
            </p>
          </div>
          <WriteReviewButton
            listingId={listingId}
            productTitle={productTitle}
          />
        </div>

        <Suspense fallback={<AiSummarySkeleton />}>
          <AiReviewSummary
            listingId={listingId}
            reviewsPromise={reviewsPromise}
          />
        </Suspense>

        <div
          data-testid="review-summary"
          className="flex flex-col gap-6 rounded-xl bg-surface-muted p-5 sm:flex-row sm:items-center"
        >
          <div className="space-y-1 text-center sm:min-w-40 sm:border-r sm:border-border-subtle sm:pr-6">
            {rated ? (
              <>
                <div className="text-2xl font-bold text-text-primary">
                  {summary.averageRating.toFixed(1)}{" "}
                  <span className="text-base font-normal text-text-secondary">
                    / 5
                  </span>
                </div>
                <Rate
                  readOnly
                  size="md"
                  value={summary.averageRating}
                  className="justify-center"
                />
                <p className="text-sm text-text-secondary">
                  {summary.reviewCount} đánh giá
                </p>
              </>
            ) : (
              <p className="text-sm text-text-secondary">Chưa có đánh giá</p>
            )}
          </div>

          <div className="min-w-0 flex-1 space-y-4">
            {rated && (
              <ul className="space-y-1.5">
                {STARS.map((star) => {
                  const count = starCount(summary.breakdown, star);
                  return (
                    <li key={star} className="flex items-center gap-3 text-sm">
                      <span className="w-12 shrink-0 text-text-secondary">
                        {star} sao
                      </span>
                      <Progress
                        className="flex-1"
                        size="sm"
                        showInfo={false}
                        percent={(count / summary.reviewCount) * 100}
                        label={`${star} sao: ${count} đánh giá`}
                      />
                      <span className="w-8 shrink-0 text-right text-text-secondary">
                        {count}
                      </span>
                    </li>
                  );
                })}
              </ul>
            )}
            <ReviewRatingFilter
              total={summary.reviewCount}
              breakdown={summary.breakdown}
              current={rating}
            />
          </div>
        </div>

        <Suspense fallback={<ReviewListSkeleton />}>
          <ReviewList
            listingId={listingId}
            productTitle={productTitle}
            pagePromise={pagePromise}
            rating={rating}
            variant={variant}
            loggedIn={loggedIn}
          />
        </Suspense>
      </Card>
    </section>
  );
}
