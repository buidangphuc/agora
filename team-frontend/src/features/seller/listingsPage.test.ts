import { beforeEach, describe, expect, it, vi } from "vitest";

import { listMyListings } from "@/lib/gateway/listings";
import { getListingsPage } from "./listingsPage";

vi.mock("@/lib/gateway/listings", () => ({ listMyListings: vi.fn() }));

function pageOf(n: number, nextCursor: string) {
  return {
    items: [{ id: `l${n}` }],
    nextCursor,
    total: 45,
  } as never;
}

beforeEach(() => vi.clearAllMocks());

describe("getListingsPage", () => {
  it("page 1 is a single call without a cursor", async () => {
    vi.mocked(listMyListings).mockResolvedValueOnce(pageOf(1, "c1"));
    const res = await getListingsPage(1);
    expect(listMyListings).toHaveBeenCalledTimes(1);
    expect(listMyListings).toHaveBeenCalledWith({ cursor: "", pageSize: 20 });
    expect(res.items[0].id).toBe("l1");
  });

  it("page 3 walks two cursors and returns the third page", async () => {
    vi.mocked(listMyListings)
      .mockResolvedValueOnce(pageOf(1, "c1"))
      .mockResolvedValueOnce(pageOf(2, "c2"))
      .mockResolvedValueOnce(pageOf(3, ""));
    const res = await getListingsPage(3);
    expect(
      vi.mocked(listMyListings).mock.calls.map((c) => c[0]?.cursor),
    ).toEqual(["", "c1", "c2"]);
    expect(res.items[0].id).toBe("l3");
  });

  it("returns an empty page (with the total) beyond the last page", async () => {
    vi.mocked(listMyListings).mockResolvedValueOnce(pageOf(1, ""));
    const res = await getListingsPage(5);
    expect(res).toEqual({ items: [], nextCursor: "", total: 45 });
    expect(listMyListings).toHaveBeenCalledTimes(1);
  });
});
