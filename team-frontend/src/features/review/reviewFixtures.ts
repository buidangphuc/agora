import type { ViewReview } from "@/lib/gateway/reviews";

export function makeReview(
  over: Partial<ViewReview> & { id: string },
): ViewReview {
  return {
    listingId: "L",
    userId: "u1",
    userName: "Nguyễn Văn A",
    orderId: "",
    rating: 5,
    comment: "Sản phẩm tốt",
    createdAt: "01/10/2026",
    mediaUrls: [],
    helpfulCount: 0,
    verifiedPurchase: false,
    ...over,
  };
}

export function makeReviews(
  count: number,
  rating = (i: number) => (i % 5) + 1,
): ViewReview[] {
  return Array.from({ length: count }, (_, i) =>
    makeReview({
      id: `r${i + 1}`,
      rating: rating(i),
      comment: `Nhận xét ${i + 1}`,
    }),
  );
}
