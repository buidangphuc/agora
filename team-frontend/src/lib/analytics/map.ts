import { readConsent } from "@/lib/consent/consent";
import { GA4EventName, InternalEventType } from "./schema";

const GA4_TO_INTERNAL: Record<GA4EventName, InternalEventType> = {
  view_item: "view",
  select_item: "click",
  view_item_list: "impression",
  add_to_cart: "add_to_cart",
  remove_from_cart: "remove_from_cart",
  view_cart: "view_cart",
  begin_checkout: "begin_checkout",
  add_shipping_info: "add_shipping_info",
  add_payment_info: "add_payment_info",
  purchase: "purchase",
  apply_promotion: "apply_promotion",
  search_filter: "search_filter",
  favorite: "favorite",
  share: "share",
};

const INTERNAL_TO_GA4: Record<InternalEventType, GA4EventName> = {
  view: "view_item",
  click: "select_item",
  impression: "view_item_list",
  add_to_cart: "add_to_cart",
  remove_from_cart: "remove_from_cart",
  view_cart: "view_cart",
  begin_checkout: "begin_checkout",
  add_shipping_info: "add_shipping_info",
  add_payment_info: "add_payment_info",
  purchase: "purchase",
  apply_promotion: "apply_promotion",
  search_filter: "search_filter",
  favorite: "favorite",
  share: "share",
};

export function ga4ToInternal(name: GA4EventName): InternalEventType {
  return GA4_TO_INTERNAL[name] ?? "view";
}

export function internalToGa4(type: InternalEventType): GA4EventName {
  return INTERNAL_TO_GA4[type] ?? "view_item";
}

const GA_COOKIE_RE = /^GA\d+\.\d+\.(\d+\.\d+)$/;
const GA_CLIENT_ID_MAX = 64;

/** Client id from the `_ga` cookie; reads the cookie only with analytics consent. */
export function readGaClientId(): string | undefined {
  try {
    if (!readConsent().analytics) return undefined;
    if (typeof document === "undefined") return undefined;
    const m = /(?:^|;\s*)_ga=([^;]*)/.exec(document.cookie);
    if (!m) return undefined;
    const id = GA_COOKIE_RE.exec(decodeURIComponent(m[1]))?.[1];
    return id && id.length <= GA_CLIENT_ID_MAX ? id : undefined;
  } catch {
    return undefined;
  }
}
