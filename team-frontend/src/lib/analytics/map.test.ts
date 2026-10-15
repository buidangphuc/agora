import { describe, expect, it } from "vitest";
import { ga4ToInternal, internalToGa4 } from "./map";
import type { InternalEventType } from "./schema";

// Spec: ga4-purchase-relay / "Browser no longer sends purchase to GA4".
// Contract: internalToGa4 returns undefined for events that must not reach
// GA4/dataLayer; ga4ToInternal keeps mapping purchase for the first-party beacon.

describe("internalToGa4 purchase", () => {
  it("[dataLayer has no purchase] internalToGa4('purchase') yields no GA4 event name", () => {
    expect(internalToGa4("purchase")).toBeUndefined();
  });

  it("does not map purchase with empty transaction_id to GA4", () => {
    // The mapping is keyed by event type only, so no transaction id (empty
    // included) can make purchase map to GA4.
    expect(internalToGa4("purchase")).not.toBe("purchase");
    expect(internalToGa4("purchase")).toBeUndefined();
  });

  it("[dataLayer has no purchase] first-party beacon type for purchase is still 'purchase'", () => {
    expect(ga4ToInternal("purchase")).toBe("purchase");
  });
});

describe("internalToGa4 other events", () => {
  const cases: Array<[InternalEventType, string]> = [
    ["view", "view_item"],
    ["click", "select_item"],
    ["impression", "view_item_list"],
    ["add_to_cart", "add_to_cart"],
    ["remove_from_cart", "remove_from_cart"],
    ["view_cart", "view_cart"],
    ["begin_checkout", "begin_checkout"],
    ["add_shipping_info", "add_shipping_info"],
    ["add_payment_info", "add_payment_info"],
    ["apply_promotion", "apply_promotion"],
    ["search_filter", "search_filter"],
    ["favorite", "add_to_wishlist"],
    ["share", "share"],
  ];
  it.each(cases)("[Other events unchanged] %s maps to %s", (internal, ga4) => {
    expect(internalToGa4(internal)).toBe(ga4);
  });
});

describe("loop events vocabulary", () => {
  it("maps loop events", () => {
    const cases: Array<[Parameters<typeof ga4ToInternal>[0], string]> = [
      ["sign_up", "sign_up"],
      ["login", "login"],
      ["add_to_wishlist", "favorite"],
      ["follow_seller", "follow_seller"],
      ["referral_redeem", "referral_redeem"],
      ["daily_check_in", "daily_check_in"],
      ["alert_subscribe", "alert_subscribe"],
      ["share", "share"],
    ];
    for (const [ga4, internal] of cases)
      expect(ga4ToInternal(ga4)).toBe(internal);
    expect(internalToGa4("favorite")).toBe("add_to_wishlist");
  });

  it("maps loop events: the unused GA4 name favorite is gone", () => {
    // @ts-expect-error "favorite" is no longer a GA4EventName
    ga4ToInternal("favorite");
  });
});
