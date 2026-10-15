import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { ReviewList } from "./ReviewList";
import { makeReview, makeReviews } from "./reviewFixtures";

vi.mock("./actions", () => ({
  markReviewHelpfulAction: vi.fn(),
  createReviewAction: vi.fn(),
  hideReviewAction: vi.fn(),
}));

// The server serves one page of `reviews` out of `total` matching reviews.
async function renderList(
  props: Partial<Parameters<typeof ReviewList>[0]> & {
    reviews: ReturnType<typeof makeReviews>;
    total?: number;
    page?: number;
  },
) {
  const { reviews, total = reviews.length, page = 1, ...rest } = props;
  const ui = await ReviewList({
    listingId: "L",
    productTitle: "Áo",
    pagePromise: Promise.resolve({
      reviews,
      total,
      page,
      pages: Math.max(1, Math.ceil(total / 10)),
    }),
    rating: 0,
    loggedIn: false,
    ...rest,
  });
  return render(ui);
}

describe("ReviewList", () => {
  it("renders the served page of 23 reviews: page 3 lists 3 and marks page 3 current", async () => {
    await renderList({ reviews: makeReviews(3), total: 23, page: 3 });
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
      reviews: makeReviews(10, () => 4),
      total: 23,
      rating: 4,
      variant: "v2",
    });
    expect(screen.getAllByTestId("review-item")).toHaveLength(10);
    expect(screen.getByRole("link", { name: "Trang 2" })).toHaveAttribute(
      "href",
      "/listing/L?variant=v2&rating=4&rpage=2#reviews",
    );
  });

  it("offers 11 pages for 105 reviews and does not slice the served page", async () => {
    await renderList({ reviews: makeReviews(5), total: 105, page: 11 });
    expect(screen.getAllByTestId("review-item")).toHaveLength(5);
    expect(screen.getByRole("link", { name: "Trang 11" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("shows an Empty with a way out when no review matches the filter", async () => {
    await renderList({
      reviews: [],
      rating: 2,
      variant: "v1",
    });
    expect(screen.queryByTestId("review-item")).toBeNull();
    expect(screen.getByText("Không có đánh giá 2 sao")).toBeInTheDocument();
    const all = screen.getByRole("link", { name: "Xem tất cả" });
    // Recovery drops `rating` (and the page), keeps the variant and the hash.
    expect(all).toHaveAttribute("href", "/listing/L?variant=v1#reviews");
  });

  it("empty state links to orders for buyer and guest", async () => {
    for (const loggedIn of [true, false]) {
      const { unmount, container } = await renderList({
        reviews: [],
        loggedIn,
      });
      expect(screen.getByText("Chưa có đánh giá nào")).toBeInTheDocument();
      expect(container.querySelector('a[href="/account/orders"]')).not.toBeNull();
      expect(container).toHaveTextContent("Để đánh giá, hãy vào");
      expect(screen.queryByRole("button", { name: "Viết đánh giá" })).toBeNull();
      expect(screen.queryByTestId("write-review")).toBeNull();
      unmount();
    }
  });

  it("badges verified and unverified reviews", async () => {
    await renderList({
      reviews: [
        makeReview({ id: "r1", verifiedPurchase: true }),
        makeReview({ id: "r2", verifiedPurchase: true }),
        makeReview({ id: "r3", verifiedPurchase: false }),
      ],
    });
    const items = screen.getAllByTestId("review-item");
    expect(items).toHaveLength(3);
    for (const item of items.slice(0, 2)) {
      expect(within(item).getByTestId("verified-purchase")).toHaveTextContent(
        "✓ Đã mua hàng",
      );
      expect(within(item).queryByTestId("unverified-purchase")).toBeNull();
    }
    expect(within(items[2]).getByTestId("unverified-purchase")).toHaveTextContent(
      "Chưa xác minh mua hàng",
    );
    expect(within(items[2]).queryByTestId("verified-purchase")).toBeNull();
  });

  it("shows hide only to admins", async () => {
    const three = () => makeReviews(3);
    const admin = await renderList({ reviews: three(), isAdmin: true });
    expect(screen.getAllByTestId("hide-review")).toHaveLength(3);
    admin.unmount();
    for (const extra of [{ isAdmin: false }, {}]) {
      const view = await renderList({
        reviews: three(),
        loggedIn: true,
        ...extra,
      });
      expect(screen.queryByTestId("hide-review")).toBeNull();
      expect(screen.queryByRole("dialog", { name: "Ẩn đánh giá" })).toBeNull();
      view.unmount();
    }
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
      "✓ Đã mua hàng",
    );
    expect(within(second).queryByTestId("verified-purchase")).toBeNull();
    expect(within(second).getByTestId("unverified-purchase")).toBeInTheDocument();
    const photos = within(first).getAllByAltText("Ảnh đánh giá");
    expect(photos).toHaveLength(2);
    for (const p of photos) expect(p).toHaveAttribute("loading", "lazy");
    expect(within(first).getByTestId("review-helpful")).toHaveTextContent(
      "Hữu ích (3)",
    );
  });
});
