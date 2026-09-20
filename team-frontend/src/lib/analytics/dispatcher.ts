import { pushDataLayer } from "./dataLayer";
import { ga4ToInternal } from "./map";
import { beaconQueue } from "./queue";
import { GA4EventName, EcommerceParams, WireTrackBeacon } from "./schema";

const ANONYMOUS_ID_KEY = "bds_anonymous_id";
const SESSION_ID_KEY = "bds_session_id";

function randomId(): string {
  try {
    if (typeof crypto !== "undefined" && crypto.randomUUID) {
      return crypto.randomUUID();
    }
  } catch {
    // Fall through
  }
  return `id-${Date.now().toString(16)}-${Math.random().toString(16).slice(2)}`;
}

function safeStorage(kind: "local" | "session"): Storage | undefined {
  try {
    return kind === "local" ? window.localStorage : window.sessionStorage;
  } catch {
    return undefined;
  }
}

function persistentId(storage: Storage | undefined, key: string): string {
  if (!storage) return "";
  try {
    const existing = storage.getItem(key);
    if (existing) return existing;
    const id = randomId();
    storage.setItem(key, id);
    return id;
  } catch {
    return "";
  }
}

/**
 * Dispatch an ecommerce interaction event across all destinations (dataLayer + in-house edge).
 * Never throws into caller. Safe during SSR.
 */
export function trackEcommerce(name: GA4EventName, params: EcommerceParams = {}): void {
  if (typeof window === "undefined") return;

  try {
    // 1. Push to window.dataLayer for GTM / GA4
    pushDataLayer(name, params);

    // 2. Build WireTrackBeacon(s) for in-house Edge Gateway (/api/track -> Kafka)
    const anonymousId = persistentId(safeStorage("local"), ANONYMOUS_ID_KEY);
    const sessionId = persistentId(safeStorage("session"), SESSION_ID_KEY);
    const path = params.path ?? (typeof location !== "undefined" ? location.pathname : "") ?? "";
    const referrer = params.referrer ?? (typeof document !== "undefined" ? document.referrer : "") ?? "";
    const query = params.query ?? "";
    const internalEventType = ga4ToInternal(name);

    const items = params.items && params.items.length > 0 ? params.items : [];

    if (items.length > 0) {
      // Fan-out: 1 row per item with shared eventGroupId (Decision D3)
      const eventGroupId = randomId();
      const beacons: WireTrackBeacon[] = items.map((it) => {
        return {
          type: internalEventType,
          listingId: it.itemId,
          sessionId,
          anonymousId,
          path,
          referrer,
          position: it.index ?? 0,
          query,
          ...(it.placementId ? { placementId: it.placementId } : {}),
          ...(it.impressionId ? { impressionId: it.impressionId } : {}),
          ...(it.modelVersion ? { modelVersion: it.modelVersion } : {}),
          ...(params.properties ? { properties: params.properties } : {}),
          ...(params.currency ? { currency: params.currency } : {}),
          ...(typeof params.value === "number" ? { value: Math.round(params.value) } : {}),
          ...(typeof it.price === "number" ? { price: Math.round(it.price) } : {}),
          ...(typeof it.quantity === "number" ? { quantity: it.quantity } : {}),
          ...(params.transactionId ? { transactionId: params.transactionId } : {}),
          ...(params.coupon ? { coupon: params.coupon } : {}),
          ...(it.itemCategory ? { itemCategory: it.itemCategory } : {}),
          ...(it.itemListId ? { itemListId: it.itemListId } : {}),
          ...(it.itemListName ? { itemListName: it.itemListName } : {}),
          eventGroupId,
          ...(params.shippingTier ? { shippingTier: params.shippingTier } : {}),
          ...(params.paymentType ? { paymentType: params.paymentType } : {}),
        };
      });

      beaconQueue.enqueueBatch(beacons);
    } else {
      // Single event (e.g. general page view or empty cart view)
      const beacon: WireTrackBeacon = {
        type: internalEventType,
        listingId: "",
        sessionId,
        anonymousId,
        path,
        referrer,
        position: 0,
        query,
        ...(params.properties ? { properties: params.properties } : {}),
        ...(params.currency ? { currency: params.currency } : {}),
        ...(typeof params.value === "number" ? { value: Math.round(params.value) } : {}),
        ...(params.transactionId ? { transactionId: params.transactionId } : {}),
        ...(params.coupon ? { coupon: params.coupon } : {}),
        ...(params.shippingTier ? { shippingTier: params.shippingTier } : {}),
        ...(params.paymentType ? { paymentType: params.paymentType } : {}),
      };
      beaconQueue.enqueue(beacon);
    }
  } catch {
    // Telemetry must never disrupt UI
  }
}
