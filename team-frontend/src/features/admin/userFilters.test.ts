import { describe, expect, it } from "vitest";

import { parseUserFilters } from "./userFilters";

// Contract under test (implementer creates src/features/admin/userFilters.ts):
//   parseUserFilters(searchParams: Record<string, string | string[] | undefined>)
//     => { query: string; status: string | undefined; cursor: string }
//   - query: first value, trimmed, cut to <= 64 UTF-8 bytes at a code-point boundary; blank -> "" (falsy)
//   - status: lowercase url value one of active|restricted|suspended|banned, else undefined
//   - cursor: first value, trimmed, capped at 256 characters; blank -> "" (falsy)

const bytes = (s: string) => new TextEncoder().encode(s).length;

describe("parseUserFilters", () => {
  it("caps query at 64 UTF-8 bytes, takes first array value, drops unknown status (Users UI: Oversized or unknown filter values are bounded)", () => {
    expect(parseUserFilters({ query: "a".repeat(300) }).query).toBe("a".repeat(64));

    const u = parseUserFilters({ query: "ư".repeat(40) }).query;
    expect(u).toBe("ư".repeat(32));
    expect(bytes(u)).toBe(64);

    expect(parseUserFilters({ query: ["first", "second"] }).query).toBe("first");

    expect(parseUserFilters({ status: "banana" }).status).toBeUndefined();
  });

  it("never splits a multi-byte character at the cap (Users UI: Oversized or unknown filter values are bounded)", () => {
    // 63 ASCII bytes + one 2-byte char would be 65 bytes: the char must be dropped whole.
    const q = parseUserFilters({ query: "a".repeat(63) + "ư" }).query;
    expect(q).toBe("a".repeat(63));
    // 4-byte code point (surrogate pair) straddling the cap.
    const e = parseUserFilters({ query: "a".repeat(62) + "😀" }).query;
    expect(e).toBe("a".repeat(62));
    expect(e).not.toContain("�");
  });

  it("keeps a query of exactly 64 bytes intact (Users UI: Oversized or unknown filter values are bounded)", () => {
    expect(parseUserFilters({ query: "a".repeat(64) }).query).toBe("a".repeat(64));
    expect(parseUserFilters({ query: "a".repeat(65) }).query).toBe("a".repeat(64));
  });

  it("trims and drops blank values (Users UI: Oversized or unknown filter values are bounded)", () => {
    const r = parseUserFilters({ query: "   ", cursor: "  ", status: " " });
    expect(r.query).toBeFalsy();
    expect(r.cursor).toBeFalsy();
    expect(r.status).toBeUndefined();
    expect(parseUserFilters({ query: "  an01  " }).query).toBe("an01");
    expect(parseUserFilters({}).query).toBeFalsy();
    expect(parseUserFilters({ query: undefined }).cursor).toBeFalsy();
  });

  it("caps cursor at 256 and takes the first array value (Users UI: Oversized or unknown filter values are bounded)", () => {
    expect(parseUserFilters({ cursor: "c".repeat(300) }).cursor).toBe("c".repeat(256));
    expect(parseUserFilters({ cursor: "c".repeat(256) }).cursor).toBe("c".repeat(256));
    expect(parseUserFilters({ cursor: ["c2", "c3"] }).cursor).toBe("c2");
  });

  it("accepts the known statuses and passes values through (Users UI: Oversized or unknown filter values are bounded)", () => {
    const r = parseUserFilters({ query: "an01", status: "suspended", cursor: "c2" });
    expect(r.query).toBe("an01");
    expect(r.status).toBe("suspended");
    expect(r.cursor).toBe("c2");
    for (const s of ["active", "restricted", "suspended", "banned"]) {
      expect(parseUserFilters({ status: s }).status).toBe(s);
    }
    expect(parseUserFilters({ status: ["banned", "active"] }).status).toBe("banned");
    expect(parseUserFilters({ status: ["banana", "active"] }).status).toBeUndefined();
  });
});
