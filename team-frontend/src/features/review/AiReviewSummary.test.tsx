import { render, screen } from "@testing-library/react";
import { StrictMode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { trackAiInteraction } from "@/lib/analytics/ai";
import { summarizeReviews } from "@/lib/gateway/ai";
import { AiReviewSummary } from "./AiReviewSummary";
import { makeReviews } from "./reviewFixtures";

vi.mock("@/lib/gateway/ai", () => ({ summarizeReviews: vi.fn() }));
vi.mock("@/lib/analytics/ai", () => ({ trackAiInteraction: vi.fn() }));

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

describe("AiReviewSummary ai_interaction", () => {
  const summary = {
    summary: "Khách hài lòng.",
    pros: ["Giá tốt"],
    cons: [],
    sentiment: "tích cực",
  };

  // Scenario: The summary card reports shown once
  it("reports shown once on mount", async () => {
    vi.mocked(summarizeReviews).mockResolvedValue(summary);
    const card = await AiReviewSummary({
      listingId: "lst_4",
      reviewsPromise: Promise.resolve(makeReviews(2)),
    });
    const { rerender } = render(<StrictMode>{card}</StrictMode>);
    expect(screen.getByTestId("ai-review-summary")).toBeInTheDocument();
    rerender(<StrictMode>{card}</StrictMode>);
    rerender(<StrictMode>{card}</StrictMode>);

    expect(vi.mocked(trackAiInteraction).mock.calls).toEqual([
      ["review_summary", "shown", "lst_4"],
    ]);
  });

  // Scenario: No summary, no event
  it("reports nothing without a summary", async () => {
    vi.mocked(summarizeReviews).mockResolvedValue(null);
    const none = await AiReviewSummary({
      listingId: "lst_4",
      reviewsPromise: Promise.resolve([]),
    });
    expect(none).toBeNull();
    render(<div>{none}</div>);

    vi.mocked(summarizeReviews).mockRejectedValue(new Error("unavailable"));
    const failed = await AiReviewSummary({
      listingId: "lst_4",
      reviewsPromise: Promise.resolve(makeReviews(1)),
    });
    render(<div>{failed}</div>);
    expect(screen.queryByTestId("ai-review-summary")).not.toBeInTheDocument();
    expect(trackAiInteraction).not.toHaveBeenCalled();
  });
});
