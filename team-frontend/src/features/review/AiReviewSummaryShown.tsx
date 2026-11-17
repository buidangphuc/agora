"use client";

import { useEffect, useRef } from "react";

import { trackAiInteraction } from "@/lib/analytics/ai";

/** Leaf client component: reports the AI review summary as shown, once. */
export function AiReviewSummaryShown({
  listingId,
  requestId,
  modelVersion,
}: {
  listingId?: string;
  requestId?: string;
  modelVersion?: string;
}) {
  const reported = useRef(false);
  useEffect(() => {
    if (reported.current) return;
    reported.current = true;
    trackAiInteraction("review_summary", "shown", listingId, {
      requestId: requestId ?? "",
      modelVersion: modelVersion ?? "",
    });
  }, [listingId, requestId, modelVersion]);
  return null;
}
