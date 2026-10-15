"use client";

import { useEffect, useRef } from "react";

import { trackAiInteraction } from "@/lib/analytics/ai";

/** Leaf client component: reports the AI review summary as shown, once. */
export function AiReviewSummaryShown({ listingId }: { listingId?: string }) {
  const reported = useRef(false);
  useEffect(() => {
    if (reported.current) return;
    reported.current = true;
    trackAiInteraction("review_summary", "shown", listingId);
  }, [listingId]);
  return null;
}
