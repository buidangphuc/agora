import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { ReviewList } from "./ReviewList";
import { makeReview, makeReviews } from "./reviewFixtures";

vi.mock("./actions", () => ({
  markReviewHelpfulAction: vi.fn(),
  createReviewAction: vi.fn(),
}));

async function renderList(
  props: Partial<Parameters<typeof ReviewList>[0]> & {
    reviews: ReturnType<typeof makeReviews>;
  },
) {
  const { reviews, ...rest } = props;
  const ui = await ReviewList({
    listingId: "L",
    productTitle: "Áo",
    reviewsPromise: Promise.resolve(reviews),
    rating: 0,
    rpage: 1,
    loggedIn: false,
    ...rest,
  });
  return render(ui);
}

describe("ReviewList", () => {
  it("paginates 23 reviews at 10 per page: page 3 lists 3 and marks page 3 current", async () => {
    await renderList({ reviews: makeReviews(23), rpage: 3 });
    expect(screen.getAllByTestId("review-item")).toHaveLength(3);
    expect(screen.getByRole("link", { name: "Trang 3" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: "Trang 2" })).toHaveAttribute(
      "href",
      "/listing/L?rpage=2#reviews",
    );
  });

  it("shows 10 on page 1 and keeps the rating filter and variant in page links", async () => {
    await renderList({
      reviews: makeReviews(23, () => 4),
      rating: 4,
      variant: "v2",
    });
    expect(screen.getAllByTestId("review-item")).toHaveLength(10);
    expect(screen.getByRole("link", { name: "Trang 2" })).toHaveAttribute(
      "href",
      "/listing/L?variant=v2&rating=4&rpage=2#reviews",
    );
  });

  it("lists only the reviews of the selected star filter", async () => {
    await renderList({ reviews: makeReviews(10), rating: 3 });
    expect(screen.getAllByTestId("review-item")).toHaveLength(2);
    for (const item of screen.getAllByTestId("review-item")) {
      expect(
        within(item).getByRole("img", { name: "3 trên 5 sao" }),
      ).toBeInTheDocument();
    }
  });

  it("shows an Empty with a way out when no review matches the filter", async () => {
    await renderList({
      reviews: makeReviews(5, () => 5),
      rating: 2,
      variant: "v1",
    });
    expect(screen.queryByTestId("review-item")).toBeNull();
    expect(screen.getByText("Không có đánh giá 2 sao")).toBeInTheDocument();
    const all = screen.getByRole("link", { name: "Xem tất cả" });
    // Recovery drops `rating` (and the page), keeps the variant and the hash.
    expect(all).toHaveAttribute("href", "/listing/L?variant=v1#reviews");
  });

  it("shows Chưa có đánh giá nào, with Viết đánh giá only for a logged-in buyer", async () => {
    const { unmount } = await renderList({ reviews: [] });
    expect(screen.getByText("Chưa có đánh giá nào")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Viết đánh giá" })).toBeNull();
    unmount();
    await renderList({ reviews: [], loggedIn: true });
    expect(
      screen.getByRole("button", { name: "Viết đánh giá" }),
    ).toBeInTheDocument();
  });

  it("renders a review with avatar, Rate, verified tag, photos and the helpful button, keeping its test ids", async () => {
    await renderList({
      reviews: [
        makeReview({
          id: "r1",
          rating: 4,
          verifiedPurchase: true,
          mediaUrls: ["http://x/p1.png", "http://x/p2.png"],
          helpfulCount: 3,
        }),
        makeReview({ id: "r2", verifiedPurchase: false }),
      ],
    });
    const [first, second] = screen.getAllByTestId("review-item");
    expect(
      within(first).getByRole("img", { name: "4 trên 5 sao" }),
    ).toBeInTheDocument();
    expect(within(first).getByTestId("verified-purchase")).toHaveTextContent(
      "Đã mua hàng",
    );
    expect(within(second).queryByTestId("verified-purchase")).toBeNull();
    const photos = within(first).getAllByAltText("Ảnh đánh giá");
    expect(photos).toHaveLength(2);
    for (const p of photos) expect(p).toHaveAttribute("loading", "lazy");
    expect(within(first).getByTestId("review-helpful")).toHaveTextContent(
      "Hữu ích (3)",
    );
  });
});
