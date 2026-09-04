import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { createReview } from "@/lib/gateway/reviews";

import { createReviewAction } from "./actions";

vi.mock("@/lib/gateway/reviews", () => ({ createReview: vi.fn() }));

beforeEach(() => vi.clearAllMocks());

describe("createReviewAction", () => {
  it("creates the review, revalidates, and returns success", async () => {
    vi.mocked(createReview).mockResolvedValue({} as never);
    const res = await createReviewAction("l1", 5, "great", "o1");
    expect(createReview).toHaveBeenCalledWith("l1", 5, "great", "o1", []);
    expect(revalidatePath).toHaveBeenCalledWith("/listing/l1");
    expect(res).toEqual({ ok: true, message: "Đánh giá sản phẩm thành công!" });
  });

  it("returns the error message when the gateway throws", async () => {
    vi.mocked(createReview).mockRejectedValue(new Error("already reviewed"));
    const res = await createReviewAction("l1", 3, "meh");
    expect(res).toEqual({ ok: false, message: "already reviewed" });
  });
});
