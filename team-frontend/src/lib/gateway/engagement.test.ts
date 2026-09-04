import { beforeEach, describe, expect, it, vi } from "vitest";

import { makeClients } from "./client.js";
import {
  addFavorite,
  getRecentlyViewed,
  getStats,
  isFavorite,
  listFavoriteIds,
  recordView,
  removeFavorite,
} from "./engagement.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

function stubEngagement(rpcs: Record<string, ReturnType<typeof vi.fn>>) {
  const engagement = {
    addFavorite: vi.fn(),
    removeFavorite: vi.fn(),
    isFavorite: vi.fn(),
    listFavorites: vi.fn(),
    recordView: vi.fn(),
    getRecentlyViewed: vi.fn(),
    getListingStats: vi.fn(),
    ...rpcs,
  };
  vi.mocked(makeClients).mockReturnValue({ engagement } as never);
  return engagement;
}

beforeEach(() => vi.clearAllMocks());

describe("engagement gateway wrapper", () => {
  it("addFavorite / removeFavorite forward the listing id", async () => {
    const engagement = stubEngagement({
      addFavorite: vi.fn().mockResolvedValue({}),
      removeFavorite: vi.fn().mockResolvedValue({}),
    });
    await addFavorite("l1");
    await removeFavorite("l1");
    expect(engagement.addFavorite).toHaveBeenCalledWith({ listingId: "l1" });
    expect(engagement.removeFavorite).toHaveBeenCalledWith({ listingId: "l1" });
  });

  it("isFavorite returns the flag on success", async () => {
    stubEngagement({
      isFavorite: vi.fn().mockResolvedValue({ favorite: true }),
    });
    await expect(isFavorite("l1")).resolves.toBe(true);
  });

  it("isFavorite defaults to false for anonymous/error", async () => {
    stubEngagement({
      isFavorite: vi.fn().mockRejectedValue(new Error("401")),
    });
    await expect(isFavorite("l1")).resolves.toBe(false);
  });

  it("listFavoriteIds maps the paged response", async () => {
    const engagement = stubEngagement({
      listFavorites: vi.fn().mockResolvedValue({
        listingIds: ["l1", "l2"],
        page: { nextCursor: "c2", total: 5n },
      }),
    });
    const res = await listFavoriteIds();
    expect(engagement.listFavorites).toHaveBeenCalledWith({
      page: { cursor: "", pageSize: 24 },
    });
    expect(res).toEqual({ ids: ["l1", "l2"], nextCursor: "c2", total: 5 });
  });

  it("recordView is best-effort and never throws", async () => {
    stubEngagement({
      recordView: vi.fn().mockRejectedValue(new Error("x")),
    });
    await expect(recordView("l1")).resolves.toBeUndefined();
  });

  it("getRecentlyViewed returns the listing ids", async () => {
    const engagement = stubEngagement({
      getRecentlyViewed: vi.fn().mockResolvedValue({ listingIds: ["l1", "l2"] }),
    });
    await expect(getRecentlyViewed()).resolves.toEqual(["l1", "l2"]);
    expect(engagement.getRecentlyViewed).toHaveBeenCalledWith({
      page: { cursor: "", pageSize: 12 },
    });
  });

  it("getRecentlyViewed defaults to [] for anonymous/error", async () => {
    stubEngagement({
      getRecentlyViewed: vi.fn().mockRejectedValue(new Error("401")),
    });
    await expect(getRecentlyViewed()).resolves.toEqual([]);
  });

  it("getStats normalizes errors to zeroed stats", async () => {
    stubEngagement({
      getListingStats: vi.fn().mockRejectedValue(new Error("x")),
    });
    await expect(getStats("l1")).resolves.toEqual({
      viewCount: 0,
      favoriteCount: 0,
    });
  });
});
