import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const text = readFileSync(join(process.cwd(), "docs/gaps.md"), "utf8");
const lines = text.split("\n");
const tableLines = lines.filter((l) => l.startsWith("|"));
const rowIds = tableLines
  .map((l) => l.split("|")[1].trim())
  .filter((id) => /^G\d+$/.test(id));
const cellCount = (l) => l.replace(/^\||\|$/g, "").split(/(?<!\\)\|/).length;

describe("gaps.md", () => {
  it("V34 gaps removed: no row with id G9, G10 or G11", () => {
    for (const id of ["G9", "G10", "G11"]) {
      expect(rowIds).not.toContain(id);
    }
  });

  it("promotion-admin-campaigns gaps removed: no row with id G12, G13 or G14", () => {
    for (const id of ["G12", "G13", "G14"]) {
      expect(rowIds).not.toContain(id);
    }
  });

  it("seller-onboarding-ui gap removed: no row with id G15", () => {
    expect(rowIds).not.toContain("G15");
  });

  it("V35 other gaps untouched: G1-G7 rows remain", () => {
    for (const n of [1, 2, 3, 4, 5, 6, 7]) {
      expect(rowIds).toContain(`G${n}`);
    }
  });

  it("V35 table format intact: every table row has the header cell count", () => {
    const expected = cellCount(tableLines[0]);
    for (const l of tableLines) {
      expect(cellCount(l)).toBe(expected);
    }
  });
});
