"use client";

import { trackEcommerce } from "@/lib/analytics";
import { useEffect, useRef } from "react";

interface TrackImpressionProps {
  readonly listingId: string;
  readonly placementId?: string;
  readonly impressionId?: string;
  readonly modelVersion?: string;
  readonly position?: number;
  readonly query?: string;
  /** Group id of the owning list instance; all its beacons share it. */
  readonly groupId?: string;
  readonly price?: number;
  readonly category?: string;
  readonly properties?: Record<string, string>;
  readonly children?: React.ReactNode;
}

type ImpressionItem = NonNullable<
  NonNullable<Parameters<typeof trackEcommerce>[1]>["items"]
>[number];

interface PendingBatch {
  query?: string;
  properties?: Record<string, string>;
  groupId?: string;
  items: ImpressionItem[];
  timer: ReturnType<typeof setTimeout>;
}

const COALESCE_MS = 300;
const pending = new Map<string, PendingBatch>();

/** Collects impressions of one list (same placement/query) into one view_item_list call. */
function enqueueImpression(
  key: string,
  query: string | undefined,
  groupId: string | undefined,
  properties: Record<string, string> | undefined,
  item: ImpressionItem,
) {
  const mapKey = `${groupId ?? ""}\u0000${key}\u0000${query ?? ""}`;
  let batch = pending.get(mapKey);
  if (!batch) {
    const created: PendingBatch = {
      query,
      properties,
      groupId,
      items: [],
      timer: setTimeout(() => {
        pending.delete(mapKey);
        try {
          trackEcommerce("view_item_list", {
            query: created.query,
            properties: created.properties,
            eventGroupId: created.groupId,
            items: created.items,
          });
        } catch {
          /* tracking must never throw */
        }
      }, COALESCE_MS),
    };
    batch = created;
    pending.set(mapKey, batch);
  }
  batch.items.push(item);
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
  groupId,
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
      enqueueImpression(placementId ?? "", query, groupId, properties, {
        itemId: listingId,
        placementId,
        impressionId,
        modelVersion,
        index: position,
        price,
        itemCategory: category,
        itemListId: placementId,
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
  }, [
    listingId,
    placementId,
    impressionId,
    modelVersion,
    position,
    query,
    groupId,
    price,
    category,
    properties,
  ]);

  if (!children) {
    return (
      <div ref={ref} className="h-0 w-0 pointer-events-none" aria-hidden />
    );
  }

  return <div ref={ref}>{children}</div>;
}
