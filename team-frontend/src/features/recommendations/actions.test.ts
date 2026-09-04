import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  RecommendationContext,
  getRecommendations,
} from "@/lib/gateway/recommendations";

import { getRecommendationsAction } from "./actions";

vi.mock("@/lib/gateway/recommendations", async () => {
  const actual = await vi.importActual<
    typeof import("@/lib/gateway/recommendations")
  >("@/lib/gateway/recommendations");
  return { ...actual, getRecommendations: vi.fn() };
});

beforeEach(() => vi.clearAllMocks());

describe("getRecommendationsAction", () => {
  it("returns the hydrated cards on success and forwards the seed + context", async () => {
    const items = [{ id: "a" }] as never;
    vi.mocked(getRecommendations).mockResolvedValue(items);
    const res = await getRecommendationsAction({
      seedListingId: "seed-1",
      context: RecommendationContext.SIMILAR_ITEMS,
    });
    expect(getRecommendations).toHaveBeenCalledWith({
      seedListingId: "seed-1",
      context: RecommendationContext.SIMILAR_ITEMS,
    });
    expect(res).toEqual({ ok: true, message: "", items });
  });

  it("degrades to an empty list with the ConnectError message on failure", async () => {
    vi.mocked(getRecommendations).mockRejectedValue(
      new ConnectError("recs off", Code.Unavailable),
    );
    const res = await getRecommendationsAction();
    expect(res.ok).toBe(false);
    expect(res.items).toEqual([]);
    expect(res.message).toContain("recs off");
  });

  it("uses a generic message for non-ConnectError failures", async () => {
    vi.mocked(getRecommendations).mockRejectedValue(new Error("boom"));
    const res = await getRecommendationsAction();
    expect(res).toEqual({
      ok: false,
      message: "Không tải được gợi ý sản phẩm.",
      items: [],
    });
  });
});
