import { beforeEach, describe, expect, it, vi } from "vitest";

import { makeClients } from "./client.js";
import {
  createReview,
  getListingRatingSummary,
  listReviews,
} from "./reviews.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

function stubEngagement(rpcs: Record<string, ReturnType<typeof vi.fn>>) {
  const engagement = {
    createReview: vi.fn(),
    listReviews: vi.fn(),
    getListingRatingSummary: vi.fn(),
    ...rpcs,
  };
  vi.mocked(makeClients).mockReturnValue({ engagement } as never);
  return engagement;
}

const protoReview = {
  id: "r1",
  listingId: "l1",
  userId: "u1",
  userName: "",
  orderId: "o1",
  rating: 5,
  comment: "great",
  createdAt: { seconds: 1_700_000_000 },
  mediaUrls: [],
  helpfulCount: 0n,
  verifiedPurchase: true,
};

beforeEach(() => vi.clearAllMocks());

describe("reviews gateway wrapper", () => {
  it("createReview maps args (default orderId) and view-maps the review", async () => {
    const engagement = stubEngagement({
      createReview: vi.fn().mockResolvedValue({ review: protoReview }),
    });
    const res = await createReview("l1", 5, "great");
    expect(engagement.createReview).toHaveBeenCalledWith({
      listingId: "l1",
      rating: 5,
      comment: "great",
      orderId: "",
      mediaUrls: [],
    });
    expect(res).toMatchObject({ id: "r1", userName: "Người mua", rating: 5 });
  });

  it("createReview throws when the gateway returns no review", async () => {
    stubEngagement({ createReview: vi.fn().mockResolvedValue({}) });
    await expect(createReview("l1", 4, "ok")).rejects.toThrow(
      "create review failed",
    );
  });

  it("listReviews normalizes errors to an empty list", async () => {
    stubEngagement({ listReviews: vi.fn().mockRejectedValue(new Error("x")) });
    await expect(listReviews("l1")).resolves.toEqual([]);
  });

  it("getListingRatingSummary maps the breakdown and averages", async () => {
    const engagement = stubEngagement({
      getListingRatingSummary: vi.fn().mockResolvedValue({
        listingId: "l1",
        averageRating: 4.2,
        reviewCount: 10n,
        breakdown: { star1: 1, star2: 0, star3: 2, star4: 3, star5: 4 },
      }),
    });
    const res = await getListingRatingSummary("l1");
    expect(engagement.getListingRatingSummary).toHaveBeenCalledWith({
      listingId: "l1",
    });
    expect(res).toMatchObject({
      averageRating: 4.2,
      reviewCount: 10,
      breakdown: { star1: 1, star5: 4 },
    });
  });

  it("getListingRatingSummary falls back to a 5-star default on error", async () => {
    stubEngagement({
      getListingRatingSummary: vi.fn().mockRejectedValue(new Error("x")),
    });
    const res = await getListingRatingSummary("l1");
    expect(res).toEqual({
      listingId: "l1",
      averageRating: 5.0,
      reviewCount: 0,
      breakdown: { star1: 0, star2: 0, star3: 0, star4: 0, star5: 0 },
    });
  });
});
