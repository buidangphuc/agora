import { describe, expect, it } from "vitest";

import { parseAuditFilters } from "./auditFilters";

// Contract under test (implementer creates src/features/admin/auditFilters.ts):
//   parseAuditFilters(searchParams: Record<string, string | string[] | undefined>)
//     => { filters: { targetType?, targetId?, actorId?, action?: string;
//                     createdAfter?, createdBefore?: Date };
//          cursor: string;                       // "" when absent
//          params: { target_type?, target_id?, actor_id?, action?, from?, to?: string };
//          error?: "invalid_date" }
// Absent filters are undefined (keys omitted or undefined), never "".

const iso = (d: Date | undefined) => d?.toISOString();

describe("parseAuditFilters", () => {
  it("maps url params to request filters (All filters are forwarded)", () => {
    const r = parseAuditFilters({
      target_type: "order",
      target_id: "o-1",
      actor_id: "a-1",
      action: "order.force_fail",
      from: "2026-10-01",
      to: "2026-10-02",
    });
    expect(r.error).toBeUndefined();
    expect(r.filters.targetType).toBe("order");
    expect(r.filters.targetId).toBe("o-1");
    expect(r.filters.actorId).toBe("a-1");
    expect(r.filters.action).toBe("order.force_fail");
    expect(iso(r.filters.createdAfter)).toBe("2026-09-30T17:00:00.000Z");
    expect(iso(r.filters.createdBefore)).toBe("2026-10-02T17:00:00.000Z");
    expect(r.cursor).toBe("");
  });

  it("omits blank values (Empty and whitespace filters are omitted)", () => {
    const r = parseAuditFilters({ target_id: "  ", action: "" });
    expect(r.error).toBeUndefined();
    expect(r.filters.targetId).toBeUndefined();
    expect(r.filters.action).toBeUndefined();
    expect(r.filters.createdAfter).toBeUndefined();
    expect(r.filters.createdBefore).toBeUndefined();
    expect(r.params.target_id).toBeUndefined();
    expect(r.params.action).toBeUndefined();
  });

  it("trims surrounding whitespace and takes the first value of an array (Empty and whitespace filters are omitted)", () => {
    const r = parseAuditFilters({
      actor_id: ["  a-1 ", "a-2"],
      target_type: " order ",
    });
    expect(r.filters.actorId).toBe("a-1");
    expect(r.filters.targetType).toBe("order");
    expect(r.params.actor_id).toBe("a-1");
  });

  it("same day is inclusive (Same day range is valid and inclusive)", () => {
    const r = parseAuditFilters({ from: "2026-10-05", to: "2026-10-05" });
    expect(r.error).toBeUndefined();
    expect(iso(r.filters.createdAfter)).toBe("2026-10-04T17:00:00.000Z");
    expect(iso(r.filters.createdBefore)).toBe("2026-10-05T17:00:00.000Z");
  });

  it("caps long values (Oversized value is capped)", () => {
    const actor = "x".repeat(500);
    const cursor = "c".repeat(600);
    const r = parseAuditFilters({ actor_id: actor, cursor });
    expect(r.filters.actorId).toBe("x".repeat(128));
    expect(r.cursor).toBe("c".repeat(256));
  });

  it("keeps a value of exactly 128 chars and caps 129 (Oversized value is capped)", () => {
    expect(
      parseAuditFilters({ action: "a".repeat(128) }).filters.action,
    ).toHaveLength(128);
    expect(
      parseAuditFilters({ action: "a".repeat(129) }).filters.action,
    ).toHaveLength(128);
  });

  it("caps target_type, target_id and action at 128 (Oversized value is capped)", () => {
    const r = parseAuditFilters({
      target_type: "t".repeat(300),
      target_id: "i".repeat(300),
      action: "a".repeat(300),
    });
    expect(r.filters.targetType).toHaveLength(128);
    expect(r.filters.targetId).toHaveLength(128);
    expect(r.filters.action).toHaveLength(128);
  });

  it("from only sets a lower bound (Only a start date is given)", () => {
    const r = parseAuditFilters({ from: "2026-10-01" });
    expect(iso(r.filters.createdAfter)).toBe("2026-09-30T17:00:00.000Z");
    expect(r.filters.createdBefore).toBeUndefined();
  });

  it("to only sets an upper bound (Only an end date is given)", () => {
    const r = parseAuditFilters({ to: "2026-10-02" });
    expect(iso(r.filters.createdBefore)).toBe("2026-10-02T17:00:00.000Z");
    expect(r.filters.createdAfter).toBeUndefined();
  });

  it("reversed range is not refused locally (End date before start date)", () => {
    const r = parseAuditFilters({ from: "2026-10-05", to: "2026-10-01" });
    expect(r.error).toBeUndefined();
    expect(iso(r.filters.createdAfter)).toBe("2026-10-04T17:00:00.000Z");
    expect(iso(r.filters.createdBefore)).toBe("2026-10-01T17:00:00.000Z");
  });

  it("day after the end date crosses a month boundary (Same day range is valid and inclusive)", () => {
    const r = parseAuditFilters({ to: "2026-10-31" });
    expect(iso(r.filters.createdBefore)).toBe("2026-10-31T17:00:00.000Z");
    const y = parseAuditFilters({ to: "2026-12-31" });
    expect(iso(y.filters.createdBefore)).toBe("2026-12-31T17:00:00.000Z");
  });

  it.each([
    ["2026-13-45"],
    ["abc"],
    ["2026-02-30"],
    ["2026-1-5"],
    ["2026-10-05T00:00:00Z"],
  ])(
    "flags %s as an invalid date (Unparseable date is refused locally)",
    (bad) => {
      expect(parseAuditFilters({ from: bad }).error).toBe("invalid_date");
      expect(parseAuditFilters({ to: bad }).error).toBe("invalid_date");
    },
  );

  it("keeps the raw date in params when invalid so the form can show it (Unparseable date is refused locally)", () => {
    const r = parseAuditFilters({ from: "abc" });
    expect(r.error).toBe("invalid_date");
    expect(r.params.from).toBe("abc");
  });
});
