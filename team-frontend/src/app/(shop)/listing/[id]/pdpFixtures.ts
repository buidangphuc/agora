import type { ViewListing, ViewVariant } from "@/lib/gateway/listings";
import type {
  ViewRatingSummary,
  ViewShopRatingSummary,
} from "@/lib/gateway/reviews";

export function variant(
  over: Partial<ViewVariant> & { id: string },
): ViewVariant {
  return {
    listingId: "L",
    name: over.id,
    sku: "",
    price: 0,
    stock: 10,
    imageUrl: "",
    ...over,
  };
}

export function makeListing(over: Partial<ViewListing> = {}): ViewListing {
  return {
    id: "L",
    title: "Điện thoại Demo",
    description: "Mô tả sản phẩm\nDòng hai",
    price: 1_000_000,
    currency: "VND",
    status: "published",
    sellerId: "seller-abcdef123",
    imageKeys: ["http://x/1.png", "http://x/2.png"],
    categoryId: "cat1",
    stock: 12,
    variants: [],
    ...over,
  };
}

export const noVariantListing = makeListing();

export const variantListing = makeListing({
  variants: [
    variant({
      id: "v1",
      name: "128GB",
      price: 1_000_000,
      stock: 12,
      sku: "SKU-128",
    }),
    variant({
      id: "v2",
      name: "256GB",
      price: 1_500_000,
      stock: 3,
      sku: "SKU-256",
    }),
    variant({
      id: "v3",
      name: "512GB",
      price: 2_000_000,
      stock: 0,
      sku: "SKU-512",
    }),
  ],
});

export const unrated: ViewRatingSummary = {
  listingId: "L",
  averageRating: 5,
  reviewCount: 0,
  breakdown: { star1: 0, star2: 0, star3: 0, star4: 0, star5: 0 },
};

export const rated: ViewRatingSummary = {
  listingId: "L",
  averageRating: 4.2,
  reviewCount: 12,
  breakdown: { star1: 0, star2: 1, star3: 1, star4: 4, star5: 6 },
};

export const shopUnrated: ViewShopRatingSummary = {
  sellerId: "seller-abcdef123",
  averageRating: 5,
  reviewCount: 0,
  breakdown: { star1: 0, star2: 0, star3: 0, star4: 0, star5: 0 },
};
