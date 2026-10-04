"use client";

import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { useToast } from "@/components/ui/ToastProvider";
import { markReviewHelpfulAction } from "./actions";

/**
 * "Hữu ích" vote. Optimistic: the count goes up at once, then takes the
 * authoritative value from the action; on failure the count reverts and an
 * error toast shows. Single use per mount (team-engagement dedupes per user).
 */
export function ReviewHelpfulButton({
  reviewId,
  listingId,
  initialCount,
}: {
  reviewId: string;
  listingId: string;
  initialCount: number;
}) {
  const [helpful, setHelpful] = useState(initialCount);
  const [voted, setVoted] = useState(false);
  const [pending, setPending] = useState(false);
  const toast = useToast();

  async function vote() {
    if (voted || pending) return;
    setVoted(true);
    setPending(true);
    setHelpful((c) => c + 1);
    try {
      const res = await markReviewHelpfulAction(reviewId, listingId);
      if (res.ok) {
        if (res.data) setHelpful(res.data.helpfulCount);
      } else {
        setVoted(false);
        setHelpful(initialCount);
        toast.error(res.error);
      }
    } catch {
      setVoted(false);
      setHelpful(initialCount);
      toast.error("Không ghi nhận được lượt hữu ích.");
    } finally {
      setPending(false);
    }
  }

  return (
    <Button
      data-testid="review-helpful"
      variant="outline"
      size="sm"
      onClick={vote}
      disabled={voted || pending}
    >
      Hữu ích ({helpful})
    </Button>
  );
}
