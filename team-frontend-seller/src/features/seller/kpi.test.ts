import { describe, expect, it } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { deriveWorkplaceKpis } from "./kpi";

const byKey = (k: ReturnType<typeof deriveWorkplaceKpis>) =>
  Object.fromEntries(k.map((x) => [x.key, x.value]));

describe("deriveWorkplaceKpis (seller-server-paging-ui: KPIs of a large catalogue)", () => {
  it("[KPIs of a large catalogue] uses the counted totals and status_counts, not page lengths", () => {
    const kpis = deriveWorkplaceKpis({
      listingsTotal: 2000,
      publishedTotal: 1200,
      statusCounts: {
        [OrderStatus.PENDING]: 7,
        [OrderStatus.PAID]: 5,
        [OrderStatus.SHIPPED]: 100,
        [OrderStatus.COMPLETED]: 900,
      },
    });
    expect(byKey(kpis)).toMatchObject({
      total: 2000,
      published: 1200,
      "to-ship": 12,
    });
    expect(kpis.find((k) => k.key === "to-ship")?.title).toBe("Cần gửi");
  });

  it("[KPIs of a large catalogue] to-ship is zero when no pending or paid count is present", () => {
    const kpis = deriveWorkplaceKpis({
      listingsTotal: 0,
      publishedTotal: 0,
      statusCounts: { [OrderStatus.COMPLETED]: 3 },
    });
    expect(byKey(kpis)["to-ship"]).toBe(0);
  });

  it("[Orders source down degrades only its blocks] omits the to-ship KPI when status_counts is null", () => {
    const kpis = deriveWorkplaceKpis({
      listingsTotal: 10,
      publishedTotal: 4,
      statusCounts: null,
    });
    expect(kpis.map((k) => k.key)).toContain("total");
    expect(kpis.map((k) => k.key)).toContain("published");
    expect(kpis.map((k) => k.key)).not.toContain("to-ship");
  });

  it("[Listings source down shows the retry state] omits listing KPIs when their counts are null and keeps to-ship", () => {
    const kpis = deriveWorkplaceKpis({
      listingsTotal: null,
      publishedTotal: null,
      statusCounts: { [OrderStatus.PAID]: 2 },
    });
    expect(kpis.map((k) => k.key)).toEqual(["to-ship"]);
    expect(
      deriveWorkplaceKpis({
        listingsTotal: null,
        publishedTotal: null,
        statusCounts: null,
      }),
    ).toEqual([]);
  });
});
