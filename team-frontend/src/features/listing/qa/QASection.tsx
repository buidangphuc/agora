import Link from "next/link";
import { Suspense } from "react";

import { Card } from "@/components/ui/Card";
import { Skeleton } from "@/components/ui/Skeleton";
import { QAAskForm } from "./QAAskForm";
import { QuestionList } from "./QuestionList";

/** Footprint of the question list while it loads. */
export function QuestionListSkeleton() {
  return (
    <div className="space-y-3">
      <Skeleton variant="text" lines={3} />
      <Skeleton variant="text" lines={3} />
    </div>
  );
}

/**
 * "Hỏi đáp" section (`#qa`, always rendered): the ask box (or a login prompt for
 * a guest) and the question list, which streams in its own Suspense boundary.
 */
export function QASection({
  listingId,
  loggedIn,
}: {
  listingId: string;
  loggedIn: boolean;
}) {
  return (
    <section id="qa" aria-labelledby="qa-heading" className="scroll-mt-40">
      <Card className="space-y-4 p-6">
        <h2 id="qa-heading" className="text-lg font-semibold text-text-primary">
          HỎI ĐÁP VỀ SẢN PHẨM (Q&amp;A)
        </h2>

        {loggedIn ? (
          <QAAskForm listingId={listingId} />
        ) : (
          <p
            data-testid="qa-login-prompt"
            className="text-sm text-text-secondary"
          >
            <Link
              href={`/login?returnUrl=/listing/${listingId}`}
              className="font-semibold text-text-primary underline"
            >
              Đăng nhập
            </Link>{" "}
            để đặt câu hỏi cho shop.
          </p>
        )}

        <Suspense fallback={<QuestionListSkeleton />}>
          <QuestionList listingId={listingId} loggedIn={loggedIn} />
        </Suspense>
      </Card>
    </section>
  );
}
