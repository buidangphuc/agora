/**
 * @deprecated Use `trackEcommerce` from `@/lib/analytics` instead.
 *
 * This file serves as a backward-compatible shim delegating to the unified
 * Analytics Dispatcher and DataLayer.
 */

import { trackEcommerce, internalToGa4 } from "./analytics";
import type { InternalEventType } from "./analytics/schema";

export type TrackEventType = InternalEventType;

export interface TrackEvent {
  readonly type: TrackEventType;
  readonly listingId?: string;
  readonly path?: string;
  readonly referrer?: string;
  readonly position?: number;
  readonly query?: string;
  readonly placementId?: string;
  readonly impressionId?: string;
  readonly modelVersion?: string;
  readonly properties?: Record<string, string>;
  readonly price?: number;
  readonly quantity?: number;
  readonly currency?: string;
  readonly value?: number;
  readonly transactionId?: string;
  readonly coupon?: string;
  readonly itemCategory?: string;
  readonly itemListId?: string;
  readonly itemListName?: string;
  readonly shippingTier?: string;
  readonly paymentType?: string;
}

export interface TrackBeacon {
  readonly type: TrackEventType;
  readonly listingId: string;
  readonly sessionId: string;
  readonly anonymousId: string;
  readonly path: string;
  readonly referrer: string;
  readonly position: number;
  readonly query: string;
  readonly placementId?: string;
  readonly impressionId?: string;
  readonly modelVersion?: string;
  readonly properties?: Record<string, string>;
}

export function track(event: TrackEvent): void {
  const ga4Name = internalToGa4(event.type);
  trackEcommerce(ga4Name, {
    path: event.path,
    referrer: event.referrer,
    query: event.query,
    properties: event.properties,
    currency: event.currency,
    value: event.value,
    transactionId: event.transactionId,
    coupon: event.coupon,
    shippingTier: event.shippingTier,
    paymentType: event.paymentType,
    items: event.listingId
      ? [
          {
            itemId: event.listingId,
            price: event.price,
            quantity: event.quantity,
            index: event.position,
            itemListId: event.itemListId,
            itemListName: event.itemListName,
            placementId: event.placementId,
            impressionId: event.impressionId,
            modelVersion: event.modelVersion,
            itemCategory: event.itemCategory,
          },
        ]
      : undefined,
  });
}
