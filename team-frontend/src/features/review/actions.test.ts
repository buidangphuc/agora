import { Code, ConnectError } from "@connectrpc/connect";
import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  createReview,
  hideReview,
  markReviewHelpful,
} from "@/lib/gateway/reviews";

import {
  createReviewAction,
  hideReviewAction,
  markReviewHelpfulAction,
} from "./actions";

vi.mock("@/lib/gateway/reviews", () => ({
  createReview: vi.fn(),
  hideReview: vi.fn(),
  markReviewHelpful: vi.fn(),
}));

beforeEach(() => vi.clearAllMocks());

const REASON_RANGE = "Lý do ẩn phải từ 1 đến 500 ký tự.";

describe("createReviewAction", () => {
  it("creates the review, revalidates the listing and orders, and returns ok", async () => {
    vi.mocked(createReview).mockResolvedValue({} as never);
    const res = await createReviewAction("l1", 5, "great", "o1");
    expect(createReview).toHaveBeenCalledWith("l1", 5, "great", "o1", []);
    expect(revalidatePath).toHaveBeenCalledWith("/listing/l1");
    expect(revalidatePath).toHaveBeenCalledWith("/account/orders");
    expect(res).toEqual({ ok: true });
  });

  it("maps AlreadyExists", async () => {
    vi.mocked(createReview).mockRejectedValue(
      new ConnectError("dup row 42", Code.AlreadyExists),
    );
    const res = await createReviewAction("l1", 5, "x", "o1");
    expect(res).toEqual({
      ok: false,
      error: "Bạn đã đánh giá sản phẩm này cho đơn hàng này.",
    });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("maps FailedPrecondition", async () => {
    vi.mocked(createReview).mockRejectedValue(
      new ConnectError("order not completed", Code.FailedPrecondition),
    );
    const res = await createReviewAction("l1", 5, "x", "o1");
    expect(res).toEqual({
      ok: false,
      error:
        "Chỉ có thể đánh giá sản phẩm trong đơn hàng đã hoàn thành của chính bạn.",
    });
  });

  it("maps Unavailable", async () => {
    vi.mocked(createReview).mockRejectedValue(
      new ConnectError("team-order down", Code.Unavailable),
    );
    const res = await createReviewAction("l1", 5, "x", "o1");
    expect(res).toEqual({
      ok: false,
      error: "Chưa xác minh được đơn hàng của bạn. Vui lòng thử lại sau.",
    });
  });

  it("maps InvalidArgument to the orders-page hint", async () => {
    vi.mocked(createReview).mockRejectedValue(
      new ConnectError("order_id required", Code.InvalidArgument),
    );
    const res = await createReviewAction("l1", 5, "x", "o1");
    expect(res).toEqual({
      ok: false,
      error: "Vui lòng đánh giá từ trang Đơn hàng của tôi.",
    });
  });

  it("maps Unauthenticated on create", async () => {
    vi.mocked(createReview).mockRejectedValue(
      new ConnectError("no token", Code.Unauthenticated),
    );
    const res = await createReviewAction("l1", 5, "x", "o1");
    expect(res).toEqual({
      ok: false,
      error: "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại.",
    });
  });

  it.each([undefined, ""])(
    "rejects an empty order id locally (%j)",
    async (orderId) => {
      const res = await createReviewAction("l1", 5, "x", orderId);
      expect(res).toEqual({
        ok: false,
        error: "Vui lòng đánh giá từ trang Đơn hàng của tôi.",
      });
      expect(createReview).not.toHaveBeenCalled();
      expect(revalidatePath).not.toHaveBeenCalled();
    },
  );

  it.each([
    ["non-Connect error", new Error("ECONNRESET")],
    ["other Connect code", new ConnectError("ECONNRESET", Code.Internal)],
    ["non-Error rejection", "ECONNRESET"],
  ])("falls back to a generic message (%s)", async (_n, err) => {
    vi.mocked(createReview).mockRejectedValue(err);
    const res = await createReviewAction("l1", 3, "meh", "o1");
    expect(res).toEqual({ ok: false, error: "Gửi đánh giá thất bại." });
    expect(JSON.stringify(res)).not.toContain("ECONNRESET");
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});

describe("hideReviewAction", () => {
  it("hides and revalidates the listing", async () => {
    vi.mocked(hideReview).mockResolvedValue(undefined as never);
    const res = await hideReviewAction("r1", "l1", "Spam quảng cáo");
    expect(hideReview).toHaveBeenCalledTimes(1);
    expect(hideReview).toHaveBeenCalledWith("r1", "Spam quảng cáo");
    expect(revalidatePath).toHaveBeenCalledWith("/listing/l1");
    expect(res).toEqual({ ok: true });
  });

  it("treats an OK reply as success", async () => {
    // Another admin hid it a moment earlier: the gateway still answers OK.
    vi.mocked(hideReview).mockResolvedValue(undefined as never);
    const res = await hideReviewAction("r1", "l1", "trùng");
    expect(res).toEqual({ ok: true });
    expect(revalidatePath).toHaveBeenCalledWith("/listing/l1");
  });

  it("trims the reason before sending", async () => {
    vi.mocked(hideReview).mockResolvedValue(undefined as never);
    await hideReviewAction("r1", "l1", "  Spam  ");
    expect(hideReview).toHaveBeenCalledWith("r1", "Spam");
  });

  it.each(["", "   "])(
    "rejects an empty reason locally (%j)",
    async (reason) => {
      const res = await hideReviewAction("r1", "l1", reason);
      expect(res).toEqual({ ok: false, error: REASON_RANGE });
      expect(hideReview).not.toHaveBeenCalled();
      expect(revalidatePath).not.toHaveBeenCalled();
    },
  );

  it("accepts 500 and rejects 501 characters", async () => {
    vi.mocked(hideReview).mockResolvedValue(undefined as never);
    const ok500 = await hideReviewAction("r1", "l1", "a".repeat(500));
    expect(ok500).toEqual({ ok: true });
    expect(hideReview).toHaveBeenCalledTimes(1);

    vi.mocked(hideReview).mockClear();
    const bad = await hideReviewAction("r1", "l1", "a".repeat(501));
    expect(bad).toEqual({ ok: false, error: REASON_RANGE });
    expect(hideReview).not.toHaveBeenCalled();
  });

  it("surfaces PermissionDenied", async () => {
    vi.mocked(hideReview).mockRejectedValue(
      new ConnectError("admin only", Code.PermissionDenied),
    );
    const res = await hideReviewAction("r1", "l1", "x");
    expect(res).toEqual({
      ok: false,
      error: "Bạn không có quyền ẩn đánh giá.",
    });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("has no role or isAdmin parameter", () => {
    expect(hideReviewAction.length).toBe(3);
  });

  it.each([
    [Code.NotFound, "Đánh giá không còn tồn tại."],
    [Code.Unavailable, "Chưa ẩn được đánh giá. Vui lòng thử lại sau."],
    [
      Code.Unauthenticated,
      "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại.",
    ],
  ])(
    "maps NotFound, Unavailable, Unauthenticated (code %s)",
    async (code, msg) => {
      vi.mocked(hideReview).mockRejectedValue(new ConnectError("raw", code));
      const res = await hideReviewAction("r1", "l1", "x");
      expect(res).toEqual({ ok: false, error: msg });
      expect(revalidatePath).not.toHaveBeenCalled();
    },
  );

  it.each([
    new ConnectError("boom detail", Code.Internal),
    new Error("boom detail"),
  ])("falls back to a generic hide message", async (err) => {
    vi.mocked(hideReview).mockRejectedValue(err);
    const res = await hideReviewAction("r1", "l1", "x");
    expect(res).toEqual({ ok: false, error: "Ẩn đánh giá thất bại." });
  });

  it("does not log the reason", async () => {
    const spies = (["log", "info", "warn", "error", "debug"] as const).map(
      (m) => vi.spyOn(console, m).mockImplementation(() => {}),
    );
    vi.mocked(hideReview).mockRejectedValue(new Error("fail"));
    await hideReviewAction("r1", "l1", "BÍ-MẬT-LÝ-DO");
    vi.mocked(hideReview).mockResolvedValue(undefined as never);
    await hideReviewAction("r1", "l1", "BÍ-MẬT-LÝ-DO");
    for (const s of spies) {
      expect(JSON.stringify(s.mock.calls)).not.toContain("BÍ-MẬT-LÝ-DO");
      s.mockRestore();
    }
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
