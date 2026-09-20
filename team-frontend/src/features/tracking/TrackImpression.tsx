"use client";

import { useEffect, useRef } from "react";
import { trackEcommerce } from "@/lib/analytics";

interface TrackImpressionProps {
  readonly listingId: string;
  readonly placementId?: string;
  readonly impressionId?: string;
  readonly modelVersion?: string;
  readonly position?: number;
  readonly query?: string;
  readonly price?: number;
  readonly category?: string;
  readonly properties?: Record<string, string>;
  readonly children?: React.ReactNode;
}

/**
 * Fires a `view_item_list` ecommerce tracking event when the element enters the viewport
 * (using IntersectionObserver), ensuring true viewability instead of DOM-only mount.
 */
export function TrackImpression({
  listingId,
  placementId,
  impressionId,
  modelVersion,
  position,
  query,
  price,
  category,
  properties,
  children,
}: TrackImpressionProps) {
  const ref = useRef<HTMLDivElement | null>(null);
  const trackedRef = useRef(false);

  useEffect(() => {
    if (trackedRef.current || !listingId) return;

    const fireTracking = () => {
      trackedRef.current = true;
      trackEcommerce("view_item_list", {
        query,
        properties,
        items: [
          {
            itemId: listingId,
            placementId,
            impressionId,
            modelVersion,
            index: position,
            price,
            itemCategory: category,
            itemListId: placementId,
          },
        ],
      });
    };

    if (typeof IntersectionObserver === "undefined") {
      fireTracking();
      return;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        const [entry] = entries;
        if (entry?.isIntersecting && !trackedRef.current) {
          fireTracking();
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
  }, [listingId, placementId, impressionId, modelVersion, position, query, price, category, properties]);

  if (!children) {
    return <div ref={ref} className="h-0 w-0 pointer-events-none" aria-hidden />;
  }

  return <div ref={ref}>{children}</div>;
}
