import { describe, expect, it } from "vitest";

import {
  FEATURED_MAX,
  TAGLINE_MAX,
  THEMES,
  addFeatured,
  moveFeatured,
  validateTagline,
  validateTheme,
} from "./shopBranding";

describe("Seller shop UI: tagline rule", () => {
  it("Seller shop UI: a 120-character tagline is accepted and 121 is refused - tagline boundary at 120", () => {
    expect(TAGLINE_MAX).toBe(120);
    expect(validateTagline("a".repeat(120))).toBeNull();
    expect(validateTagline("a".repeat(121))).toBe("Tagline tối đa 120 ký tự.");
  });

  it("Seller shop UI: a 120-character tagline is accepted - counts code points after trimming", () => {
    expect(validateTagline(`  ${"😀".repeat(120)}  `)).toBeNull();
    expect(validateTagline("😀".repeat(121))).toBe("Tagline tối đa 120 ký tự.");
  });

  it("Seller shop UI: an empty tagline is valid", () => {
    expect(validateTagline("")).toBeNull();
    expect(validateTagline("   ")).toBeNull();
  });

  it.each([["a\nb"], ["a\u0000b"], ["a\tb"], ["a\u007fb"]])(
    "Seller shop UI: a control character in the tagline is refused - tagline control characters %j",
    (value) => {
      expect(validateTagline(value)).toBe(
        "Tagline không được chứa ký tự điều khiển.",
      );
    },
  );

  it("Seller shop UI: tagline control characters - Vietnamese text passes", () => {
    expect(validateTagline("Hoa tươi mỗi ngày")).toBeNull();
  });
});

describe("Seller shop UI: theme set", () => {
  it("offers exactly default, dark and festive", () => {
    expect([...THEMES]).toEqual(["default", "dark", "festive"]);
    for (const t of THEMES) expect(validateTheme(t)).toBeNull();
    expect(validateTheme("warm")).not.toBeNull();
    expect(validateTheme("")).not.toBeNull();
  });
});

describe("Seller shop UI: featured cap", () => {
  const ids = (n: number) => Array.from({ length: n }, (_, i) => `l${i}`);

  it("Seller shop UI: The sixth featured listing is accepted - sixth accepted", () => {
    expect(FEATURED_MAX).toBe(6);
    const res = addFeatured(ids(5), "l5");
    expect(res).toEqual({ ok: true, ids: ids(6) });
  });

  it("Seller shop UI: The seventh featured listing is refused - cap of six", () => {
    const res = addFeatured(ids(6), "x");
    expect(res).toEqual({ ok: false, error: "Tối đa 6 sản phẩm nổi bật." });
  });

  it("Seller shop UI: an already selected listing is not added twice - no duplicates", () => {
    expect(addFeatured(["a"], "a")).toEqual({ ok: true, ids: ["a"] });
  });

  it("Seller shop UI: reordering changes the saved order - moveFeatured", () => {
    expect(moveFeatured(["a", "b", "c"], 2, -1)).toEqual(["a", "c", "b"]);
    expect(moveFeatured(["a", "b", "c"], 0, -1)).toEqual(["a", "b", "c"]);
    expect(moveFeatured(["a", "b", "c"], 2, 1)).toEqual(["a", "b", "c"]);
  });
});
