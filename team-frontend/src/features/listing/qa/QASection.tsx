import { Link } from "@/components/ui/Link";
import { loginPath } from "@/features/auth/returnUrlShared";
import { Suspense } from "react";

import { Card } from "@/components/ui/Card";
import { Skeleton } from "@/components/ui/Skeleton";
import { QAAskForm } from "./QAAskForm";
import { QuestionList } from "./QuestionList";

/** Footprint of the question list while it loads. */
export function QuestionListSkeleton() {
  return (
    <div className="space-y-4">
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
      <Card padded>
        <div className="space-y-6">
          <h2
            id="qa-heading"
            className="text-xl font-semibold tracking-tight text-text-primary"
          >
            HỎI ĐÁP VỀ SẢN PHẨM (Q&amp;A)
          </h2>

          {loggedIn ? (
            <QAAskForm listingId={listingId} />
          ) : (
            <p
              data-testid="qa-login-prompt"
              className="rounded-2xl bg-surface-muted px-5 py-4 text-sm text-text-secondary"
            >
              <span className="font-semibold">
                <Link href={loginPath(`/listing/${listingId}`)}>Đăng nhập</Link>
              </span>{" "}
              để đặt câu hỏi cho shop.
            </p>
          )}

          <Suspense fallback={<QuestionListSkeleton />}>
            <QuestionList listingId={listingId} loggedIn={loggedIn} />
          </Suspense>
        </div>
      </Card>
    </section>
  );
}
