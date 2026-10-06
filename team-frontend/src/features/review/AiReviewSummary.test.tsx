import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { summarizeReviews } from "@/lib/gateway/ai";
import { AiReviewSummary } from "./AiReviewSummary";
import { makeReviews } from "./reviewFixtures";

vi.mock("@/lib/gateway/ai", () => ({ summarizeReviews: vi.fn() }));

beforeEach(() => vi.clearAllMocks());

describe("AiReviewSummary", () => {
  it("renders nothing when the summary call rejects (team-ai UNAVAILABLE)", async () => {
    vi.mocked(summarizeReviews).mockRejectedValue(new Error("unavailable"));
    const out = await AiReviewSummary({
      listingId: "L",
      reviewsPromise: Promise.resolve(makeReviews(3)),
    });
    expect(out).toBeNull();
  });

  it("renders nothing when team-ai returns no summary or the reviews fail to load", async () => {
    vi.mocked(summarizeReviews).mockResolvedValue(null);
    expect(
      await AiReviewSummary({
        listingId: "L",
        reviewsPromise: Promise.resolve([]),
      }),
    ).toBeNull();
    expect(
      await AiReviewSummary({
        listingId: "L",
        reviewsPromise: Promise.reject(new Error("boom")),
      }),
    ).toBeNull();
  });

  it("sends the shared reviews to the summary call and renders the card", async () => {
    vi.mocked(summarizeReviews).mockResolvedValue({
      summary: "Khách hài lòng.",
      pros: ["Giá tốt"],
      cons: ["Giao chậm"],
      sentiment: "tích cực",
    });
    const reviews = makeReviews(2);
    render(
      await AiReviewSummary({
        listingId: "L",
        reviewsPromise: Promise.resolve(reviews),
      }),
    );
    expect(summarizeReviews).toHaveBeenCalledWith("L", [
      { rating: reviews[0].rating, comment: reviews[0].comment },
      { rating: reviews[1].rating, comment: reviews[1].comment },
    ]);
    expect(screen.getByText("Tóm tắt đánh giá bằng AI")).toBeInTheDocument();
    expect(screen.getByText("Khách hài lòng.")).toBeInTheDocument();
    expect(screen.getByText("• Giá tốt")).toBeInTheDocument();
    expect(screen.getByText("• Giao chậm")).toBeInTheDocument();
  });
});
