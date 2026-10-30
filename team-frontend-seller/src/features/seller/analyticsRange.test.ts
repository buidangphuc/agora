import { describe, expect, it } from "vitest";

import { daysInRange, parseRange, ratio } from "./analyticsRange";

const NOW = Date.parse("2026-10-10T00:00:00Z");
const day = (d: string) => ({ day: d, revenue: 1, orderCount: 1 });

describe("parseRange", () => {
  it("accepts the three ranges and defaults everything else", () => {
    expect(parseRange("30d")).toBe("30d");
    expect(parseRange("90d")).toBe("90d");
    expect(parseRange("7d")).toBe("7d");
    expect(parseRange("bogus")).toBe("7d");
    expect(parseRange(undefined)).toBe("7d");
    expect(parseRange(["90d", "7d"])).toBe("90d");
  });
});

describe("daysInRange", () => {
  const days = [
    day("2026-10-09"),
    day("2026-10-01"),
    day("2026-09-20"),
    day("2026-07-20"),
  ];
  it("keeps only the days inside the range", () => {
    expect(daysInRange(days, "7d", NOW).map((d) => d.day)).toEqual([
      "2026-10-09",
    ]);
    expect(daysInRange(days, "30d", NOW).map((d) => d.day)).toEqual([
      "2026-10-09",
      "2026-10-01",
      "2026-09-20",
    ]);
    expect(daysInRange(days, "90d", NOW)).toHaveLength(4);
  });

  it("keeps a day it cannot parse rather than dropping data", () => {
    expect(daysInRange([day("n/a")], "7d", NOW)).toHaveLength(1);
  });
});

describe("ratio", () => {
  it("is a percentage of impressions, safe at zero", () => {
    expect(ratio(50, 200)).toBe(25);
    expect(ratio(5, 0)).toBe(0);
    expect(ratio(300, 200)).toBe(100);
  });
});
