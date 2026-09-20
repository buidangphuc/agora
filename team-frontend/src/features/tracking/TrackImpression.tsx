"use client";

import { useEffect, useRef } from "react";

import { track } from "@/lib/track";

interface TrackImpressionProps {
  readonly listingId: string;
  readonly placementId?: string;
  readonly impressionId?: string;
  readonly modelVersion?: string;
  readonly position?: number;
  readonly query?: string;
  readonly properties?: Record<string, string>;
  readonly children?: React.ReactNode;
}

/**
 * Fires an `impression` tracking event when the element enters the viewport
 * (using IntersectionObserver), ensuring true viewability instead of DOM-only mount.
 */
export function TrackImpression({
  listingId,
  placementId,
  impressionId,
  modelVersion,
  position,
  query,
  properties,
  children,
}: TrackImpressionProps) {
  const ref = useRef<HTMLDivElement | null>(null);
  const trackedRef = useRef(false);

  useEffect(() => {
    if (trackedRef.current || !listingId) return;

    if (typeof IntersectionObserver === "undefined") {
      // Fallback if IntersectionObserver is not available
      track({
        type: "impression",
        listingId,
        placementId,
        impressionId,
        modelVersion,
        position,
        query,
        properties,
      });
      trackedRef.current = true;
      return;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        const [entry] = entries;
        if (entry?.isIntersecting && !trackedRef.current) {
          trackedRef.current = true;
          track({
            type: "impression",
            listingId,
            placementId,
            impressionId,
            modelVersion,
            position,
            query,
            properties,
          });
          observer.disconnect();
        }
      },
      { threshold: 0.3 },
    );

    const el = ref.current;
    if (el) {
      observer.observe(el);
    }

    return () => {
      observer.disconnect();
    };
  }, [listingId, placementId, impressionId, modelVersion, position, query, properties]);

  if (!children) {
    return <div ref={ref} className="h-0 w-0 pointer-events-none" aria-hidden />;
  }

  return <div ref={ref}>{children}</div>;
}
