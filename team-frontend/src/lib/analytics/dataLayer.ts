import { GA4EventName, EcommerceParams } from "./schema";

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
export function pushDataLayer(name: GA4EventName, params: EcommerceParams): void {
  if (typeof window === "undefined") return;

  try {
    window.dataLayer = window.dataLayer || [];

    // 1. Reset ecommerce object to avoid merging residual properties
    window.dataLayer.push({ ecommerce: null });

    // 2. Format ecommerce payload
    const ecommercePayload: Record<string, unknown> = {};
    if (params.currency) ecommercePayload.currency = params.currency;
    if (typeof params.value === "number") ecommercePayload.value = params.value;
    if (params.coupon) ecommercePayload.coupon = params.coupon;
    if (params.transactionId) ecommercePayload.transaction_id = params.transactionId;
    if (params.shippingTier) ecommercePayload.shipping_tier = params.shippingTier;
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
    window.dataLayer.push({
      event: name,
      ecommerce: ecommercePayload,
      ...(params.path ? { page_path: params.path } : {}),
      ...(params.referrer ? { page_referrer: params.referrer } : {}),
      ...(params.query ? { search_term: params.query } : {}),
      ...(params.properties ? params.properties : {}),
    });
  } catch {
    // Telemetry must never crash the UI
  }
}
