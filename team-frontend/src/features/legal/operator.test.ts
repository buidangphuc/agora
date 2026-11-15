import { describe, expect, it } from "vitest";

import { OPERATOR_FIELDS } from "./operator";

describe("operator block", () => {
  it("[Footer shows the operator placeholders] has four labelled placeholder fields", () => {
    expect(OPERATOR_FIELDS).toHaveLength(4);
    for (const f of OPERATOR_FIELDS) {
      expect(f.label.length).toBeGreaterThan(0);
      expect(f.value).toBe("[DO BỘ PHẬN PHÁP CHẾ CUNG CẤP]");
    }
    expect(new Set(OPERATOR_FIELDS.map((f) => f.label)).size).toBe(4);
  });
});
