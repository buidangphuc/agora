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
