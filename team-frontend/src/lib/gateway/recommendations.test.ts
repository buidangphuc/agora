import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { RecommendationContext } from "@/generated/platform/recommendation/v1/recommendation_pb.js";

import { makeClients } from "./client.js";
import { getListing } from "./listings.js";
import { getRecommendations } from "./recommendations.js";
import { getPrincipal } from "./session.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./listings.js", () => ({ getListing: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  getPrincipal: vi.fn(() => ({ id: "u1", name: "Buyer", scopes: [] })),
  SESSION_COOKIE: "session",
}));

function stubRecommend(fn: ReturnType<typeof vi.fn>) {
  vi.mocked(makeClients).mockReturnValue({
    recommendation: { recommend: fn },
  } as never);
  return fn;
}

function card(id: string, stock = 10) {
  return {
    id,
    title: `Listing ${id}`,
    price: 1000,
    status: "published",
    stock,
  };
}

beforeEach(() => vi.clearAllMocks());

describe("getRecommendations", () => {
  it("forwards the caller id + context and hydrates ids into cards in rank order", async () => {
    const recommend = stubRecommend(
      vi.fn().mockResolvedValue({
        items: [
          { listingId: "b", score: 0.4, rank: 2 },
          { listingId: "a", score: 0.9, rank: 1 },
        ],
        modelVersion: "als-2026-09",
      }),
    );
    vi.mocked(getListing).mockImplementation(
      async (id: string) => card(id) as never,
    );

    const items = await getRecommendations({
      seedListingId: "seed-1",
      context: RecommendationContext.SIMILAR_ITEMS,
      limit: 5,
    });

    // Over-fetch: requested 5 cards, ask team-ai for 5 * 3 = 15 ids.
    expect(recommend).toHaveBeenCalledWith({
      userId: "u1",
      anonymousId: "",
      seedListingId: "seed-1",
      context: RecommendationContext.SIMILAR_ITEMS,
      limit: 15,
    });
    // Best-first by rank: a (rank 1) then b (rank 2).
    expect(items.map((l) => l.id)).toEqual(["a", "b"]);
  });

  it("drops ids that no longer resolve to a listing", async () => {
    stubRecommend(
      vi.fn().mockResolvedValue({
        items: [
          { listingId: "a", score: 1, rank: 1 },
          { listingId: "gone", score: 1, rank: 2 },
        ],
        modelVersion: "v1",
      }),
    );
    vi.mocked(getListing).mockImplementation(async (id: string) =>
      id === "gone" ? null : (card(id) as never),
    );

    const items = await getRecommendations();
    expect(items.map((l) => l.id)).toEqual(["a"]);
  });

  it("returns an empty list when the service is UNAVAILABLE (row hidden)", async () => {
    stubRecommend(
      vi.fn().mockRejectedValue(new ConnectError("recs off", Code.Unavailable)),
    );
    await expect(getRecommendations()).resolves.toEqual([]);
    expect(getListing).not.toHaveBeenCalled();
  });

  it("uses an empty user id for an anonymous caller and defaults limit to 10", async () => {
    vi.mocked(getPrincipal).mockReturnValueOnce(null);
    const recommend = stubRecommend(
      vi.fn().mockResolvedValue({ items: [], modelVersion: "" }),
    );
    await getRecommendations();
    // Default limit 10 → over-fetch asks team-ai for 10 * 3 = 30 ids.
    expect(recommend).toHaveBeenCalledWith({
      userId: "",
      anonymousId: "",
      seedListingId: "",
      context: RecommendationContext.UNSPECIFIED,
      limit: 30,
    });
  });

  it("propagates non-UNAVAILABLE errors", async () => {
    stubRecommend(
      vi.fn().mockRejectedValue(new ConnectError("boom", Code.Internal)),
    );
    await expect(getRecommendations()).rejects.toThrow("boom");
  });

  it("drops out-of-stock listings and preserves rank order", async () => {
    stubRecommend(
      vi.fn().mockResolvedValue({
        items: [
          { listingId: "a", score: 0.9, rank: 1 },
          { listingId: "sold", score: 0.8, rank: 2 },
          { listingId: "c", score: 0.7, rank: 3 },
        ],
        modelVersion: "v1",
      }),
    );
    // "sold" hydrates but has zero stock → dropped; order otherwise preserved.
    vi.mocked(getListing).mockImplementation(async (id: string) =>
      id === "sold" ? (card(id, 0) as never) : (card(id) as never),
    );

    const items = await getRecommendations({ limit: 5 });
    expect(items.map((l) => l.id)).toEqual(["a", "c"]);
  });

  it("over-fetches and trims to exactly `limit` in-stock cards when enough exist", async () => {
    // limit 2 → over-fetch asks for 6; team-ai returns 6 ids, one sold out.
    const recommend = stubRecommend(
      vi.fn().mockResolvedValue({
        items: [
          { listingId: "r1", score: 0.9, rank: 1 },
          { listingId: "sold", score: 0.85, rank: 2 },
          { listingId: "r3", score: 0.8, rank: 3 },
          { listingId: "r4", score: 0.7, rank: 4 },
          { listingId: "r5", score: 0.6, rank: 5 },
          { listingId: "r6", score: 0.5, rank: 6 },
        ],
        modelVersion: "v1",
      }),
    );
    vi.mocked(getListing).mockImplementation(async (id: string) =>
      id === "sold" ? (card(id, 0) as never) : (card(id) as never),
    );

    const items = await getRecommendations({ limit: 2 });
    expect(recommend).toHaveBeenCalledWith(
      expect.objectContaining({ limit: 6 }),
    );
    // Best-first, sold-out skipped, trimmed to exactly 2.
    expect(items.map((l) => l.id)).toEqual(["r1", "r3"]);
  });

  it("returns fewer than `limit` when not enough in-stock listings exist", async () => {
    stubRecommend(
      vi.fn().mockResolvedValue({
        items: [
          { listingId: "a", score: 0.9, rank: 1 },
          { listingId: "sold1", score: 0.8, rank: 2 },
          { listingId: "sold2", score: 0.7, rank: 3 },
        ],
        modelVersion: "v1",
      }),
    );
    vi.mocked(getListing).mockImplementation(async (id: string) =>
      id.startsWith("sold") ? (card(id, 0) as never) : (card(id) as never),
    );

    const items = await getRecommendations({ limit: 5 });
    expect(items.map((l) => l.id)).toEqual(["a"]);
  });

  it("caps the over-fetch so a large limit never explodes the id request", async () => {
    const recommend = stubRecommend(
      vi.fn().mockResolvedValue({ items: [], modelVersion: "" }),
    );
    await getRecommendations({ limit: 50 });
    // 50 * 3 = 150, capped at 60.
    expect(recommend).toHaveBeenCalledWith(
      expect.objectContaining({ limit: 60 }),
    );
  });
});
