import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { addToCartAction } from "@/features/cart/actions";
import { trackEcommerce } from "@/lib/analytics";
import { getCategory, getListing, getStorefront } from "@/lib/gateway/listings";
import { getActiveFlashSale } from "@/lib/gateway/promotion";
import { getRecommendations } from "@/lib/gateway/recommendations";
import {
  getListingRatingSummary,
  getShopRatingSummary,
  listReviews,
  listReviewsPage,
} from "@/lib/gateway/reviews";
import { setupUser } from "@/test/user";
import ProductDetailPage from "./page";
import {
  makeListing,
  shopUnrated,
  unrated,
  variantListing,
} from "./pdpFixtures";

vi.mock("next/navigation", () => ({
  notFound: () => {
    throw new Error("NEXT_NOT_FOUND");
  },
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  usePathname: () => "/listing/L",
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/lib/gateway/listings", () => ({
  getListing: vi.fn(),
  getCategory: vi.fn(),
  getStorefront: vi.fn(),
}));
vi.mock("@/lib/gateway/engagement", () => ({
  isFavorite: vi.fn().mockResolvedValue(false),
  listCollections: vi.fn().mockResolvedValue([]),
  recordView: vi.fn().mockResolvedValue(undefined),
}));
vi.mock("@/lib/gateway/notification", () => ({
  AlertType: { PRICE_DROP: 1, BACK_IN_STOCK: 2 },
  listAlertSubscriptions: vi.fn().mockResolvedValue([]),
}));
vi.mock("@/lib/gateway/promotion", () => ({ getActiveFlashSale: vi.fn() }));
vi.mock("@/lib/gateway/reviews", () => ({
  getListingRatingSummary: vi.fn(),
  getShopRatingSummary: vi.fn(),
  listReviews: vi.fn(),
  listReviewsPage: vi.fn(),
}));
vi.mock("@/lib/gateway/recommendations", () => ({
  getRecommendations: vi.fn(),
  RecommendationContext: { HOMEPAGE: 1, SIMILAR_ITEMS: 2 },
}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn(() => null) }));
vi.mock("@/features/cart/actions", () => ({ addToCartAction: vi.fn() }));
vi.mock("@/lib/analytics", () => ({ trackEcommerce: vi.fn() }));

// Not under test here: their own client behaviour, and the async children that
// stream in their own Suspense boundaries.
vi.mock("@/components/alerts/AlertToggle", () => ({ AlertToggle: () => null }));
vi.mock("@/features/engagement/AddToCollectionButton", () => ({
  AddToCollectionButton: () => null,
}));
vi.mock("@/features/engagement/FavoriteButton", () => ({
  FavoriteButton: () => null,
}));
vi.mock("@/features/listing/ShareButton", () => ({ ShareButton: () => null }));
vi.mock("@/features/chat/ChatWithSellerButton", () => ({
  ChatWithSellerButton: () => null,
}));
vi.mock("@/features/listing/LiveFlashSaleStock", () => ({
  LiveFlashSaleStock: () => null,
}));
vi.mock("@/features/review/ReviewList", () => ({ ReviewList: () => null }));
vi.mock("@/features/review/AiReviewSummary", () => ({
  AiReviewSummary: () => null,
}));
vi.mock("@/features/listing/qa/QuestionList", () => ({
  QuestionList: () => null,
}));
// The page renders the async row inside Suspense; stub it there and test the
// real row below.
vi.mock("@/features/recommendations/RecommendationsRow", () => ({
  RecommendationsRow: () => null,
}));

const { RecommendationsRow } = await vi.importActual<
  typeof import("@/features/recommendations/RecommendationsRow")
>("@/features/recommendations/RecommendationsRow");

const withRecs = (items: ReturnType<typeof makeListing>[]) => ({
  items,
  requestId: "req-srv-1",
  placementId: "",
  modelVersion: "m1",
});

function setupPage(listing = makeListing({ price: 100_000 })) {
  vi.mocked(getListing).mockResolvedValue(listing);
  vi.mocked(getCategory).mockResolvedValue(null);
  vi.mocked(getStorefront).mockResolvedValue(null);
  vi.mocked(getListingRatingSummary).mockResolvedValue(unrated);
  vi.mocked(getShopRatingSummary).mockResolvedValue(shopUnrated);
  vi.mocked(getActiveFlashSale).mockResolvedValue({ active: false });
  vi.mocked(listReviews).mockResolvedValue([]);
  vi.mocked(listReviewsPage).mockResolvedValue({
    reviews: [],
    total: 0,
    page: 1,
    pages: 1,
  });
  vi.mocked(getRecommendations).mockResolvedValue(withRecs([]));
}

beforeEach(() => {
  vi.clearAllMocks();
});

const eventsOf = (name: string) =>
  vi.mocked(trackEcommerce).mock.calls.filter(([n]) => n === name);

/** Render the page minus the async recommendations row (rendered separately). */
async function renderPdp(
  listing = makeListing({ price: 100_000 }),
  searchParams: Record<string, string> = {},
) {
  setupPage(listing);
  return render(await ProductDetailPage({ params: { id: "L" }, searchParams }));
}

describe("PDP tracking: view_item", () => {
  it("fires exactly one view_item on load, none on a variant change or anchor navigation", async () => {
    const user = setupUser();
    await renderPdp(variantListing);

    expect(eventsOf("view_item")).toHaveLength(1);
    expect(eventsOf("view_item")[0][1]).toMatchObject({
      path: "/listing/L",
      items: [{ itemId: "L" }],
    });

    await user.click(screen.getByRole("radio", { name: "256GB" }));
    await user.click(screen.getByRole("link", { name: "Đánh giá" }));
    await user.click(screen.getByRole("link", { name: "Hỏi đáp" }));

    expect(eventsOf("view_item")).toHaveLength(1);
    expect(eventsOf("select_item")).toHaveLength(0);
  });
});

describe("PDP tracking: add_to_cart", () => {
  it("sends one add_to_cart with the same payload shape: value, VND, item id, name, price, quantity, category", async () => {
    const user = setupUser();
    vi.mocked(addToCartAction).mockResolvedValue({ ok: true });
    await renderPdp(
      makeListing({ price: 100_000, title: "Áo thun", categoryId: "cat9" }),
    );

    await user.click(screen.getByRole("button", { name: "Tăng số lượng" }));
    await user.click(screen.getByTestId("pdp-add-to-cart"));

    expect(eventsOf("add_to_cart")).toHaveLength(1);
    expect(eventsOf("add_to_cart")[0][1]).toEqual({
      currency: "VND",
      value: 200_000,
      items: [
        {
          itemId: "L",
          itemName: "Áo thun",
          price: 100_000,
          quantity: 2,
          itemCategory: "cat9",
        },
      ],
    });
  });
});

describe("PDP tracking: similar-items attribution", () => {
  const recs = [
    makeListing({ id: "R1", title: "Gợi ý 1" }),
    makeListing({ id: "R2", title: "Gợi ý 2" }),
    makeListing({ id: "R3", title: "Gợi ý 3" }),
  ];

  it("seeds the row with the listing, keeps ListingGrid placement similar_items and fires impressions at positions 1..n", async () => {
    vi.mocked(getRecommendations).mockResolvedValue(withRecs(recs));
    const { container } = render(
      await RecommendationsRow({ seedListingId: "L" }),
    );

    expect(
      container
        .querySelector("section[data-recs-request-id]")
        ?.getAttribute("data-recs-request-id"),
    ).toBe("req-srv-1");
    expect(getRecommendations).toHaveBeenCalledWith(
      expect.objectContaining({ seedListingId: "L", context: 2 }),
    );
    const impressions = eventsOf("view_item_list").map(
      ([, p]) => p?.items?.[0],
    );
    expect(impressions.map((i) => i?.itemId)).toEqual(["R1", "R2", "R3"]);
    expect(impressions.map((i) => i?.index)).toEqual([1, 2, 3]);
    for (const i of impressions) {
      expect(i?.placementId).toBe("similar_items");
      expect(i?.itemListId).toBe("similar_items");
      expect(i?.impressionId).toBe("req-srv-1");
    }
  });

  it("fires select_item with the placement and position when a card is clicked", async () => {
    const user = setupUser();
    vi.mocked(getRecommendations).mockResolvedValue(withRecs(recs));
    const { container } = render(
      await RecommendationsRow({ seedListingId: "L" }),
    );
    const link = container.querySelector(
      'a[href="/listing/R2"]',
    ) as HTMLAnchorElement;
    // jsdom cannot navigate; the click handler still runs.
    link.addEventListener("click", (e) => e.preventDefault());
    await user.click(link);
    expect(eventsOf("select_item")).toHaveLength(1);
    expect(eventsOf("select_item")[0][1]?.items?.[0]).toMatchObject({
      itemId: "R2",
      placementId: "similar_items",
      index: 2,
      itemListId: "similar_items",
    });
  });

  it("renders nothing when the recommendation service fails or returns no items", async () => {
    vi.mocked(getRecommendations).mockRejectedValue(new Error("UNAVAILABLE"));
    expect(await RecommendationsRow({ seedListingId: "L" })).toBeNull();
    vi.mocked(getRecommendations).mockResolvedValue(withRecs([]));
    expect(await RecommendationsRow({ seedListingId: "L" })).toBeNull();
  });
});
