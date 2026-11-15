import { Code, ConnectError } from "@connectrpc/connect";
import { describe, expect, it } from "vitest";

import {
  BULK_SHIP_MAX,
  NOT_ATTEMPTED_REASON,
  parseIds,
  reasonForError,
  runPool,
  toggleSelection,
} from "./bulkShip";

const ids = (n: number) => Array.from({ length: n }, (_, i) => `o${i + 1}`);

describe("parseIds", () => {
  it("splits, trims and dedupes keeping first-seen order", () => {
    expect(parseIds(" a , b,a,,c ")).toEqual({ ok: true, ids: ["a", "b", "c"] });
  });

  it("accepts exactly 50 distinct ids", () => {
    const res = parseIds(ids(50).join(","));
    expect(res.ok).toBe(true);
  });

  it("Bulk ship: more than 50 print ids are rejected (51 distinct)", () => {
    const res = parseIds(ids(51).join(","));
    expect(res).toEqual({ ok: false, error: "Chọn tối đa 50 đơn" });
  });

  it("51 raw ids with duplicates collapsing to 50 is accepted", () => {
    const raw = [...ids(50), "o1"].join(",");
    expect(parseIds(raw).ok).toBe(true);
  });

  it("parseIds rejects empty and malformed", () => {
    for (const raw of [undefined, null, "", "   ", ",", " , ,", "a b", "a;b", "a/b", "<x>"]) {
      expect(parseIds(raw as string | undefined).ok).toBe(false);
    }
  });
});

describe("selection cap", () => {
  it("selection stops at 50", () => {
    const full = ids(50);
    const res = toggleSelection(full, "extra");
    expect(res.selected).toEqual(full);
    expect(res.error).toBe("Chọn tối đa 50 đơn");
  });

  it("adds below the cap, removes a selected id even at the cap", () => {
    expect(toggleSelection(["a"], "b")).toEqual({ selected: ["a", "b"] });
    const removed = toggleSelection(ids(50), "o3");
    expect(removed.selected).toHaveLength(49);
    expect(removed.selected).not.toContain("o3");
    expect(removed.error).toBeUndefined();
  });

  it("exports the cap", () => {
    expect(BULK_SHIP_MAX).toBe(50);
  });
});

describe("reasonForError", () => {
  it.each([
    [Code.PermissionDenied, "Không có quyền với đơn này"],
    [Code.NotFound, "Không tìm thấy đơn"],
    [
      Code.FailedPrecondition,
      "Trạng thái đơn không cho phép gửi (có thể đã huỷ hoặc đã gửi)",
    ],
    [Code.Unavailable, "Dịch vụ đơn hàng đang bận"],
    [Code.DeadlineExceeded, "Dịch vụ đơn hàng đang bận"],
    [Code.Internal, "Cập nhật thất bại"],
  ])("maps connect code %s", (code, text) => {
    expect(reasonForError(new ConnectError("raw upstream text", code))).toBe(text);
  });

  it("never leaks raw messages for non-connect errors", () => {
    expect(reasonForError(new Error("secret sql"))).toBe("Cập nhật thất bại");
    expect(reasonForError("boom")).toBe("Cập nhật thất bại");
  });

  it("not-attempted reason text", () => {
    expect(NOT_ATTEMPTED_REASON).toBe("Chưa xử lý, hãy thử lại");
  });
});

describe("runPool", () => {
  it("runner caps concurrency at 4 and keeps order", async () => {
    let inFlight = 0;
    let max = 0;
    const releases: Array<() => void> = [];
    const items = ids(50);
    const p = runPool(items, async (id) => {
      inFlight++;
      max = Math.max(max, inFlight);
      await new Promise<void>((r) => releases.push(r));
      inFlight--;
      return id.toUpperCase();
    });
    for (let spins = 0; spins < 5000; spins++) {
      await Promise.resolve();
      if (releases.length) releases.shift()?.();
      else if (inFlight === 0 && spins > 100) break;
    }
    const out = await p;
    expect(max).toBe(4);
    expect(out).toHaveLength(50);
    out.forEach((r, i) => {
      expect(r).toEqual({ done: true, value: items[i].toUpperCase() });
    });
  });

  it("deadline marks unstarted as not attempted", async () => {
    let clock = 0;
    const started: string[] = [];
    const out = await runPool(
      ids(14),
      async (id) => {
        started.push(id);
        clock = 15_000;
        return id;
      },
      { now: () => clock },
    );
    expect(started).toHaveLength(4);
    expect(out.slice(0, 4).every((r) => r.done)).toBe(true);
    expect(out.slice(4)).toEqual(Array(10).fill({ done: false }));
  });

  it("does not skip work before the deadline", async () => {
    let clock = 0;
    const out = await runPool(
      ids(10),
      async (id) => {
        clock += 1_000;
        return id;
      },
      { now: () => clock },
    );
    expect(out.every((r) => r.done)).toBe(true);
  });

  it("an empty list yields an empty result", async () => {
    expect(await runPool([], async () => 1)).toEqual([]);
  });
});
