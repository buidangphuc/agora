import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * Source guards for the order screens (ui-phase-orders 5.1 / 5.2): no tracking
 * hook or data-* attribute other than test ids, the e2e test ids kept, and no
 * emoji status icons.
 */
const roots = ["src/features/order", "src/app/(shop)/account/orders"];
// Checkout files in this folder belong to ui-phase-cart-checkout and legitimately carry
// checkout tracking (begin_checkout, data-checkout-shell); they are not order screens.
const EXCLUDE =
  /(\.test\.tsx?$|CheckoutView|SellerOrdersList|orderFixtures|BeginCheckoutBeacon|CheckoutPending|CheckoutStepper|CheckoutSteps|MobileSummaryBar|OrderSummary|PaymentOptionsGrid|PlaceOrderForm|checkoutParams|orderTotal|paymentOptions|shipping)/;

function sources(): { file: string; text: string }[] {
  const out: { file: string; text: string }[] = [];
  const walk = (dir: string) => {
    for (const name of readdirSync(dir)) {
      const full = join(dir, name);
      if (statSync(full).isDirectory()) walk(full);
      else if (/\.tsx?$/.test(name) && !EXCLUDE.test(name)) {
        out.push({ file: full, text: readFileSync(full, "utf8") });
      }
    }
  };
  for (const r of roots) walk(join(process.cwd(), r));
  return out;
}

const REQUIRED_TEST_IDS = [
  "order-timeline",
  "timeline-checkpoint",
  "timeline-saga",
  "timeline-saga-step",
  "timeline-empty",
  "timeline-failure",
  "return-section",
  "return-reason",
  "return-amount",
  "return-submit",
  "return-status",
];

describe("order screens source guards", () => {
  const files = sources();
  const all = files.map((f) => f.text).join("\n");

  it("adds no tracking hooks", () => {
    expect(all).not.toMatch(
      /TrackLink|TrackImpression|SearchImpressions|AnalyticsProvider|placement/,
    );
  });

  it("uses no data-* attribute other than data-testid", () => {
    const attrs = [...all.matchAll(/\bdata-([a-z-]+)=/g)].map((m) => m[1]);
    expect(attrs.filter((a) => a !== "testid")).toEqual([]);
  });

  it("offers the buyer no refund control", () => {
    expect(all).not.toContain('"return-refund"');
    expect(all).not.toMatch(new RegExp(["mock", "RefundAction"].join("")));
  });

  it("keeps every existing e2e test id", () => {
    for (const id of REQUIRED_TEST_IDS) {
      expect(all, id).toContain(`"${id}"`);
    }
  });

  it("has no emoji status icons or arbitrary type sizes", () => {
    expect(all).not.toMatch(/\p{Extended_Pictographic}/u);
    expect(all).not.toMatch(/text-\[\d+px\]|rounded-2xs/);
  });
});
