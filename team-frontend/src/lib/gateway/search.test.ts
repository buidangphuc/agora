import { beforeEach, describe, expect, it, vi } from "vitest";

import { SortBy } from "@/generated/platform/search/v1/search_pb.js";

import { makeClients } from "./client.js";
import { getListing } from "./listings.js";
import { SEARCH_MAX_PAGE, searchListings } from "./search.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "t"),
  SESSION_COOKIE: "session",
}));
vi.mock("./listings.js", () => ({ getListing: vi.fn() }));

const rpc = vi.fn();

function pageOf(n: number, nextCursor: string, total = 60n) {
  return {
    hits: [{ listingId: `p${n}-a` }, { listingId: `p${n}-b` }],
    page: { nextCursor, total },
    facets: undefined,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(makeClients).mockReturnValue({
    search: { searchListings: rpc },
  } as never);
  vi.mocked(getListing).mockImplementation(
    async (id: string) => ({ id, title: id, status: "published" }) as never,
  );
});

describe("searchListings paging", () => {
  it("returns page 1 with one call, page size 24 and the total", async () => {
    rpc.mockResolvedValueOnce(pageOf(1, "c1"));
    const res = await searchListings("ao", { sortBy: SortBy.NEWEST });
    expect(rpc).toHaveBeenCalledTimes(1);
    expect(rpc.mock.calls[0]?.[0]).toMatchObject({
      query: "ao",
      sortBy: SortBy.NEWEST,
      page: { cursor: "", pageSize: 24 },
    });
    expect(res.page).toBe(1);
    expect(res.total).toBe(60);
    expect(res.items.map((l) => l.id)).toEqual(["p1-a", "p1-b"]);
  });

  it("walks next_cursor to page 3", async () => {
    rpc
      .mockResolvedValueOnce(pageOf(1, "c1"))
      .mockResolvedValueOnce(pageOf(2, "c2"))
      .mockResolvedValueOnce(pageOf(3, ""));
    const res = await searchListings("ao", { page: 3 });
    expect(rpc).toHaveBeenCalledTimes(3);
    expect(rpc.mock.calls[1]?.[0].page).toEqual({ cursor: "c1", pageSize: 24 });
    expect(rpc.mock.calls[2]?.[0].page).toEqual({ cursor: "c2", pageSize: 24 });
    expect(res.page).toBe(3);
    expect(res.items.map((l) => l.id)).toEqual(["p3-a", "p3-b"]);
  });

  it("returns the last existing page when the page is out of range", async () => {
    rpc
      .mockResolvedValueOnce(pageOf(1, "c1"))
      .mockResolvedValueOnce(pageOf(2, ""));
    const res = await searchListings("ao", { page: 99 });
    expect(rpc).toHaveBeenCalledTimes(2);
    expect(res.page).toBe(2);
  });

  it("caps the walk at the maximum page", async () => {
    rpc.mockImplementation(async () => pageOf(0, "more"));
    const res = await searchListings("ao", { page: 999 });
    expect(rpc).toHaveBeenCalledTimes(SEARCH_MAX_PAGE);
    expect(res.page).toBe(SEARCH_MAX_PAGE);
  });

  it("treats an invalid page as 1", async () => {
    rpc.mockResolvedValue(pageOf(1, ""));
    const res = await searchListings("ao", { page: Number.NaN });
    expect(res.page).toBe(1);
    expect(rpc).toHaveBeenCalledTimes(1);
  });

  it("derives a lower-bound total when the backend reports -1", async () => {
    rpc.mockResolvedValueOnce(pageOf(1, "c1", -1n));
    const res = await searchListings("ao");
    expect(res.total).toBe(3);
  });

  it("lets failures propagate", async () => {
    rpc.mockRejectedValueOnce(new Error("search down"));
    await expect(searchListings("ao")).rejects.toThrow("search down");
  });
});
