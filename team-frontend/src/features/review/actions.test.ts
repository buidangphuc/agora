import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { createReview, markReviewHelpful } from "@/lib/gateway/reviews";

import { createReviewAction, markReviewHelpfulAction } from "./actions";

vi.mock("@/lib/gateway/reviews", () => ({
  createReview: vi.fn(),
  markReviewHelpful: vi.fn(),
}));

beforeEach(() => vi.clearAllMocks());

describe("createReviewAction", () => {
  it("creates the review, revalidates the listing and orders, and returns ok", async () => {
    vi.mocked(createReview).mockResolvedValue({} as never);
    const res = await createReviewAction("l1", 5, "great", "o1");
    expect(createReview).toHaveBeenCalledWith("l1", 5, "great", "o1", []);
    expect(revalidatePath).toHaveBeenCalledWith("/listing/l1");
    expect(revalidatePath).toHaveBeenCalledWith("/account/orders");
    expect(res).toEqual({ ok: true });
  });

  it("returns error when the gateway throws, without revalidating", async () => {
    vi.mocked(createReview).mockRejectedValue(new Error("already reviewed"));
    const res = await createReviewAction("l1", 3, "meh");
    expect(res).toEqual({ ok: false, error: "already reviewed" });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("uses a non-empty fallback error for a non-Error rejection", async () => {
    vi.mocked(createReview).mockRejectedValue("boom");
    const res = await createReviewAction("l1", 3, "meh");
    expect(res).toEqual({ ok: false, error: "Gửi đánh giá thất bại." });
  });
});

describe("markReviewHelpfulAction", () => {
  it("returns the new count and revalidates the listing page", async () => {
    vi.mocked(markReviewHelpful).mockResolvedValue(7);
    const res = await markReviewHelpfulAction("r1", "L");
    expect(markReviewHelpful).toHaveBeenCalledWith("r1");
    expect(revalidatePath).toHaveBeenCalledWith("/listing/L");
    expect(res).toEqual({ ok: true, data: { helpfulCount: 7 } });
  });

  it("returns error when the vote is rejected", async () => {
    vi.mocked(markReviewHelpful).mockRejectedValue(new Error("already voted"));
    const res = await markReviewHelpfulAction("r1", "L");
    expect(res).toEqual({ ok: false, error: "already voted" });
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});
