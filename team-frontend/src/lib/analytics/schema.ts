/**
 * GA4 / Ecommerce Analytics Types & Schemas
 */

export type GA4EventName =
  | "view_item"
  | "select_item"
  | "view_item_list"
  | "add_to_cart"
  | "remove_from_cart"
  | "view_cart"
  | "begin_checkout"
  | "add_shipping_info"
  | "add_payment_info"
  | "purchase"
  | "apply_promotion"
  | "search_filter"
  | "favorite"
  | "share";

export type InternalEventType =
  | "view"
  | "click"
  | "add_to_cart"
  | "impression"
  | "remove_from_cart"
  | "begin_checkout"
  | "apply_promotion"
  | "search_filter"
  | "favorite"
  | "share"
  | "view_cart"
  | "add_shipping_info"
  | "add_payment_info"
  | "purchase";

export interface EcommerceItem {
  itemId: string;
  itemName?: string;
  itemCategory?: string;
  price?: number; // Major units (e.g. 50000 VND)
  quantity?: number;
  index?: number; // 1-based position in list
  itemListId?: string;
  itemListName?: string;
  placementId?: string;
  impressionId?: string;
  modelVersion?: string;
  sellerId?: string;
}

export interface EcommerceParams {
  currency?: string;
  value?: number;
  coupon?: string;
  transactionId?: string;
  shippingTier?: string;
  paymentType?: string;
  items?: EcommerceItem[];
  path?: string;
  referrer?: string;
  query?: string;
  properties?: Record<string, string>;
}

/** Wire format sent to gateway collector /api/track */
export interface WireTrackBeacon {
  type: string;
  listingId: string;
  sessionId: string;
  anonymousId: string;
  path: string;
  referrer: string;
  position: number;
  query: string;
  placementId?: string;
  impressionId?: string;
  modelVersion?: string;
  properties?: Record<string, string>;
  currency?: string;
  value?: number; // minor units (e.g. cents/VND)
  price?: number; // minor units
  quantity?: number;
  transactionId?: string;
  coupon?: string;
  itemCategory?: string;
  itemListId?: string;
  itemListName?: string;
  eventGroupId?: string;
  shippingTier?: string;
  paymentType?: string;
}
