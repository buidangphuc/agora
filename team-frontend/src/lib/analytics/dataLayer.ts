import { readConsent } from "@/lib/consent/consent";
import { EcommerceParams, GA4EventName } from "./schema";
import { scrubReferrer, scrubText } from "./scrub";

/** Non-personal caller property keys allowed into the data layer. */
const ALLOWED_PROPERTIES = ["valid"] as const;

declare global {
  interface Window {
    dataLayer?: Array<Record<string, unknown>>;
  }
}

/**
 * Push an event to window.dataLayer following GA4 Enhanced Ecommerce standard.
 * Resets { ecommerce: null } immediately prior to every push to avoid property pollution.
 * Safe during SSR; never throws.
 */
export function pushDataLayer(
  name: GA4EventName,
  params: EcommerceParams,
): void {
  if (typeof window === "undefined") return;

  try {
    // No analytics event reaches the dataLayer without analytics consent.
    if (!readConsent().analytics) return;

    window.dataLayer = window.dataLayer || [];

    // 1. Reset ecommerce object to avoid merging residual properties
    window.dataLayer.push({ ecommerce: null });

    // 2. Format ecommerce payload
    const ecommercePayload: Record<string, unknown> = {};
    if (params.currency) ecommercePayload.currency = params.currency;
    if (typeof params.value === "number") ecommercePayload.value = params.value;
    if (params.coupon) ecommercePayload.coupon = params.coupon;
    if (params.transactionId)
      ecommercePayload.transaction_id = params.transactionId;
    if (params.shippingTier)
      ecommercePayload.shipping_tier = params.shippingTier;
    if (params.paymentType) ecommercePayload.payment_type = params.paymentType;

    if (params.items && params.items.length > 0) {
      ecommercePayload.items = params.items.map((it) => ({
        item_id: it.itemId,
        ...(it.itemName ? { item_name: it.itemName } : {}),
        ...(it.itemCategory ? { item_category: it.itemCategory } : {}),
        ...(typeof it.price === "number" ? { price: it.price } : {}),
        ...(typeof it.quantity === "number" ? { quantity: it.quantity } : {}),
        ...(typeof it.index === "number" ? { index: it.index } : {}),
        ...(it.itemListId ? { item_list_id: it.itemListId } : {}),
        ...(it.itemListName ? { item_list_name: it.itemListName } : {}),
        ...(it.placementId ? { placement_id: it.placementId } : {}),
      }));
    }

    // 3. Push complete event
    const props: Record<string, string> = {};
    for (const k of ALLOWED_PROPERTIES) {
      const v = params.properties?.[k];
      if (typeof v === "string") props[k] = scrubText(v);
    }
    const referrer = params.referrer ? scrubReferrer(params.referrer) : "";

    window.dataLayer.push({
      event: name,
      ecommerce: ecommercePayload,
      ...(params.path ? { page_path: scrubText(params.path) } : {}),
      ...(referrer ? { page_referrer: referrer } : {}),
      ...(params.query ? { search_term: scrubText(params.query) } : {}),
      ...(Object.keys(props).length > 0 ? { properties: props } : {}),
    });
  } catch {
    // Telemetry must never crash the UI
  }
}
