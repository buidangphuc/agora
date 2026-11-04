import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const text = readFileSync(new URL("../docs/gaps.md", import.meta.url), "utf8");
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

  it("V35 other gaps untouched: G1-G8 and G12-G15 rows remain", () => {
    for (const n of [1, 2, 3, 4, 5, 6, 7, 8, 12, 13, 14, 15]) {
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
