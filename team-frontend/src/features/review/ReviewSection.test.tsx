import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { ViewRatingSummary } from "@/lib/gateway/reviews";
import { ReviewSection } from "./ReviewSection";

// The async children stream in their own Suspense boundaries; stub them so the
// synchronous shell (heading, summary box, filter) can be rendered here.
vi.mock("./ReviewList", () => ({
  ReviewList: () => <div data-testid="list" />,
}));
vi.mock("./AiReviewSummary", () => ({ AiReviewSummary: () => null }));
vi.mock("./ReviewRatingFilter", () => ({
  ReviewRatingFilter: ({
    total,
    current,
  }: { total: number; current: number }) => (
    <div data-testid="filter" data-total={total} data-current={current} />
  ),
}));

const rated: ViewRatingSummary = {
  listingId: "L",
  averageRating: 3.5,
  reviewCount: 20,
  breakdown: { star1: 2, star2: 3, star3: 5, star4: 4, star5: 6 },
};
const unrated: ViewRatingSummary = {
  listingId: "L",
  averageRating: 5,
  reviewCount: 0,
  breakdown: { star1: 0, star2: 0, star3: 0, star4: 0, star5: 0 },
};

function renderSection(
  summary: ViewRatingSummary,
  rating = 0,
  loggedIn = true,
) {
  return render(
    <ReviewSection
      listingId="L"
      productTitle="Áo"
      summary={summary}
      reviewsPromise={Promise.resolve([])}
      pagePromise={Promise.resolve({
        reviews: [],
        total: 0,
        page: 1,
        pages: 1,
      })}
      rating={rating}
      loggedIn={loggedIn}
    />,
  );
}

describe("ReviewSection", () => {
  it("labels the count as verified purchases", () => {
    renderSection({ ...rated, averageRating: 4.0, reviewCount: 2 });
    const box = screen.getByTestId("review-summary");
    expect(box).toHaveTextContent("4.0");
    expect(box).toHaveTextContent("từ 2 đánh giá đã mua hàng");
    expect(box.textContent).not.toMatch(/\b2 đánh giá(?! đã mua hàng)/);
  });

  it("is the always-rendered #reviews section with the kept heading", () => {
    renderSection(rated);
    const section = document.getElementById("reviews");
    expect(section).not.toBeNull();
    expect(
      screen.getByRole("heading", { level: 2, name: "ĐÁNH GIÁ SẢN PHẨM" }),
    ).toBeInTheDocument();
  });

  it("renders the orders hint and no write button", () => {
    renderSection(rated);
    const section = document.getElementById("reviews") as HTMLElement;
    expect(within(section).queryByTestId("write-review")).toBeNull();
    expect(
      within(section).queryByRole("button", { name: "Viết đánh giá" }),
    ).toBeNull();
    const link = section.querySelector('a[href="/account/orders"]');
    expect(link).not.toBeNull();
    expect(section).toHaveTextContent("Để đánh giá, hãy vào");
  });

  it("renders the orders hint and no write button for a guest too", () => {
    renderSection(rated, 0, false);
    const section = document.getElementById("reviews") as HTMLElement;
    expect(within(section).queryByTestId("write-review")).toBeNull();
    expect(section.querySelector('a[href="/account/orders"]')).not.toBeNull();
  });

  it("shows the average with a read-only Rate, the count and the star breakdown as Progress", () => {
    renderSection(rated, 4);
    const box = screen.getByTestId("review-summary");
    expect(box).toHaveTextContent("3.5");
    expect(box).toHaveTextContent("/ 5");
    expect(
      within(box).getByRole("img", { name: "3.5 trên 5 sao" }),
    ).toBeInTheDocument();
    const five = within(box).getByRole("progressbar", {
      name: "5 sao: 6 đánh giá",
    });
    expect(five).toHaveAttribute("aria-valuenow", "30");
    expect(within(box).getAllByRole("progressbar")).toHaveLength(5);
    expect(screen.getByTestId("filter")).toHaveAttribute("data-current", "4");
    expect(screen.getByTestId("filter")).toHaveAttribute("data-total", "20");
  });

  it("keeps the empty summary text: only Chưa có đánh giá, with no stars and no invented 5.0, for zero reviews", () => {
    renderSection(unrated);
    const box = screen.getByTestId("review-summary");
    expect(box).toHaveTextContent("Chưa có đánh giá");
    expect(box).not.toHaveTextContent("từ 0 đánh giá đã mua hàng");
    expect(box).not.toHaveTextContent("5.0");
    expect(within(box).queryByRole("img")).toBeNull();
    expect(within(box).queryAllByRole("progressbar")).toHaveLength(0);
  });
});
