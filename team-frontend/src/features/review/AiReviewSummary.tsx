import { Card } from "@/components/ui/Card";
import { Tag } from "@/components/ui/Tag";
import { type ViewReviewSummary, summarizeReviews } from "@/lib/gateway/ai";
import type { ViewReview } from "@/lib/gateway/reviews";

/** The AI summary card. Presentational. */
export function AiReviewSummaryCard({
  summary,
}: {
  summary: ViewReviewSummary;
}) {
  return (
    <Card data-testid="ai-review-summary" className="p-5">
      <h3 className="flex flex-wrap items-center gap-2 text-sm font-semibold text-text-primary">
        <span>Tóm tắt đánh giá bằng AI</span>
        {summary.sentiment && <Tag color="info">{summary.sentiment}</Tag>}
      </h3>

      {summary.summary && (
        <p className="mt-2 text-sm leading-relaxed text-text-secondary">
          {summary.summary}
        </p>
      )}

      <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
        {summary.pros.length > 0 && (
          <div>
            <p className="text-sm font-semibold text-accent-success-dark">
              Ưu điểm
            </p>
            <ul className="mt-1 space-y-0.5 text-sm text-text-secondary">
              {summary.pros.map((p) => (
                <li key={p}>• {p}</li>
              ))}
            </ul>
          </div>
        )}
        {summary.cons.length > 0 && (
          <div>
            <p className="text-sm font-semibold text-danger">Hạn chế</p>
            <ul className="mt-1 space-y-0.5 text-sm text-text-secondary">
              {summary.cons.map((c) => (
                <li key={c}>• {c}</li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </Card>
  );
}

/**
 * AI-generated summary of a listing's reviews (team-ai SummarizeReviews via the
 * gateway). An async server component rendered inside a Suspense boundary, so a
 * slow team-ai never delays the page's first byte. Renders nothing when the
 * listing has no reviews or the service is unavailable or fails.
 */
export async function AiReviewSummary({
  listingId,
  reviewsPromise,
}: {
  listingId: string;
  /** Shared with the review list so the reviews are fetched once. */
  reviewsPromise: Promise<ViewReview[]>;
}) {
  const summary = await reviewsPromise
    .then((reviews) =>
      summarizeReviews(
        listingId,
        reviews.map((r) => ({ rating: r.rating, comment: r.comment })),
      ),
    )
    .catch(() => null);
  if (!summary) return null;
  return <AiReviewSummaryCard summary={summary} />;
}
