import Link from "next/link";
import { notFound } from "next/navigation";

import { AlertToggle } from "@/components/alerts/AlertToggle";
import { formatPrice } from "@/components/ui/format";
import { AddToCartButton } from "@/features/cart/AddToCartButton";
import { ChatWithSellerButton } from "@/features/chat/ChatWithSellerButton";
import { AddToCollectionButton } from "@/features/engagement/AddToCollectionButton";
import { FavoriteButton } from "@/features/engagement/FavoriteButton";
import { LiveFlashSaleStock } from "@/features/listing/LiveFlashSaleStock";
import { ShareButton } from "@/features/listing/ShareButton";
import { QASection } from "@/features/listing/qa/QASection";
import { RecommendationsRow } from "@/features/recommendations/RecommendationsRow";
import { AiReviewSummary } from "@/features/review/AiReviewSummary";
import { ReviewSection } from "@/features/review/ReviewSection";
import { ShopRatingSummary } from "@/features/review/ShopRatingSummary";
import { TrackView } from "@/features/tracking/TrackView";
import {
  listCollections,
  listQuestionsByListing,
  recordView,
} from "@/lib/gateway/engagement";
import { getListing } from "@/lib/gateway/listings";
import { AlertType, listAlertSubscriptions } from "@/lib/gateway/notification";
import { getActiveFlashSale } from "@/lib/gateway/promotion";
import { summarizeReviews } from "@/lib/gateway/ai";
import {
  getListingRatingSummary,
  getShopRatingSummary,
  listReviews,
} from "@/lib/gateway/reviews";
import { getPrincipal } from "@/lib/gateway/session";
import { getImageUrl } from "@/lib/media";

export const dynamic = "force-dynamic";

export default async function ProductDetailPage({
  params,
}: {
  params: { id: string };
}) {
  const listing = await getListing(params.id);
  if (!listing) notFound();

  const me = getPrincipal();
  const loggedIn = me !== null;

  // Server-fetch every engagement/notification slot in parallel (gateway-only).
  // The client component polls live remaining stock; null = not on flash sale.
  const [
    flashSale,
    reviews,
    ratingSummary,
    shopSummary,
    collections,
    alertSubs,
    questions,
  ] = await Promise.all([
    getActiveFlashSale(params.id),
    listReviews(params.id),
    getListingRatingSummary(params.id),
    getShopRatingSummary(listing.sellerId),
    loggedIn ? listCollections() : Promise.resolve([]),
    loggedIn ? listAlertSubscriptions() : Promise.resolve([]),
    listQuestionsByListing(params.id),
    // Best-effort: record this view so it shows up in "Vừa xem" (recently
    // viewed). Never throws; its result is ignored.
    recordView(params.id),
  ]);

  const flashSaleCampaign =
    flashSale.active && flashSale.campaign ? flashSale.campaign : null;

  // AI review summary (team-ai via the gateway) — null when no reviews or the
  // service is unavailable, in which case the block is hidden.
  const aiReviewSummary = await summarizeReviews(
    params.id,
    reviews.map((r) => ({ rating: r.rating, comment: r.comment })),
  );

  // Existing alert subscriptions for this listing, keyed by type, so the
  // toggles reflect current state.
  const listingSubs = alertSubs.filter((s) => s.listingId === params.id);
  const alertState = {
    priceDropSubId: listingSubs.find((s) => s.type === AlertType.PRICE_DROP)
      ?.id,
    backInStockSubId: listingSubs.find(
      (s) => s.type === AlertType.BACK_IN_STOCK,
    )?.id,
  };

  const images =
    listing.imageKeys && listing.imageKeys.length > 0
      ? listing.imageKeys.map(getImageUrl)
      : [
          listing.imageUrl ||
            "https://images.unsplash.com/photo-1523275335684-37898b6baf30?w=800",
        ];

  const isMall =
    listing.price > 5000000 ||
    listing.title.toLowerCase().includes("chính hãng") ||
    listing.title.toLowerCase().includes("apple") ||
    listing.title.toLowerCase().includes("sony") ||
    listing.title.toLowerCase().includes("philips") ||
    listing.title.toLowerCase().includes("nike");

  const originalPrice = Math.round(listing.price * 1.25);

  return (
    <div className="space-y-4">
      {/* Fire a best-effort VIEW tracking beacon on PDP mount. */}
      <TrackView listingId={listing.id} path={`/listing/${listing.id}`} />

      {/* ── Breadcrumb ── */}
      <nav className="flex items-center gap-2 text-xs text-gray-500">
        <Link href="/" className="hover:text-brand">
          Trang Chủ
        </Link>
        <span>&gt;</span>
        <Link href="/search" className="hover:text-brand">
          Danh mục sản phẩm
        </Link>
        <span>&gt;</span>
        <span className="truncate max-w-md font-semibold text-gray-800">
          {listing.title}
        </span>
      </nav>

      {/* ── Main Product Box ── */}
      <div className="rounded-xs bg-white p-6 shadow-2xs">
        <div className="grid grid-cols-1 gap-8 md:grid-cols-12">
          {/* Left Column: Photos Gallery */}
          <div className="md:col-span-5 space-y-3">
            <div className="relative aspect-square w-full overflow-hidden rounded-xs border bg-gray-50">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={images[0]}
                alt={listing.title}
                className="h-full w-full object-cover"
              />
              <div className="absolute top-2 left-2 z-10">
                {isMall ? (
                  <span className="rounded-2xs bg-[#d0011b] px-2 py-0.5 text-[10px] font-black text-white uppercase shadow-xs">
                    Chính Hãng
                  </span>
                ) : (
                  <span className="rounded-2xs bg-brand px-2 py-0.5 text-[10px] font-bold text-white uppercase shadow-xs">
                    Yêu thích+
                  </span>
                )}
              </div>
              <div className="absolute bottom-2 right-2">
                <FavoriteButton id={listing.id} initial={false} />
              </div>
            </div>

            {/* Thumbnail Strip */}
            {images.length > 1 && (
              <div className="flex gap-2 overflow-x-auto">
                {images.map((img, idx) => (
                  <div
                    key={img}
                    className="h-16 w-16 shrink-0 overflow-hidden rounded-xs border-2 border-transparent hover:border-brand cursor-pointer transition"
                  >
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={img}
                      alt={`Thumb ${idx + 1}`}
                      className="h-full w-full object-cover"
                    />
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Right Column: Title, Price, Buy Actions */}
          <div className="md:col-span-7 space-y-4">
            {/* Title */}
            <h1 className="text-lg sm:text-xl font-medium text-gray-900 leading-snug">
              {isMall && (
                <span className="mr-2 inline-block rounded-2xs bg-[#d0011b] px-1.5 py-0.5 text-[10px] font-bold text-white uppercase">
                  Mall
                </span>
              )}
              {listing.title}
            </h1>

            {/* Ratings & Sold Stats */}
            <div className="flex items-center gap-4 text-xs text-gray-500 border-b pb-3 flex-wrap">
              <div className="flex items-center gap-1 border-r pr-4">
                <span className="font-bold text-brand text-sm underline">
                  5.0
                </span>
                <span className="text-yellow-400">★★★★★</span>
              </div>
              <div className="border-r pr-4">
                <span className="font-semibold text-gray-800">125</span> Đánh
                Giá
              </div>
              <div>
                <span className="font-semibold text-gray-800">1.2k</span> Đã Bán
              </div>
            </div>

            {/* Share short link (team-sharing via gateway) */}
            <div className="flex items-center justify-end pt-1">
              <ShareButton id={listing.id} />
            </div>

            {/* Price Box */}
            <div className="rounded-xs bg-[#fafafa] p-4 flex items-center gap-4 flex-wrap">
              <span className="text-xs text-gray-400 line-through">
                {formatPrice(originalPrice, listing.currency)}
              </span>
              <span className="text-2xl sm:text-3xl font-bold text-brand">
                {formatPrice(listing.price, listing.currency)}
              </span>
              <span className="rounded-2xs bg-brand px-1.5 py-0.5 text-[10px] font-bold text-white uppercase">
                20% GIẢM
              </span>
            </div>

            {/* 🔥 Live Flash Sale — real campaign from team-promotion (gateway).
                Renders nothing when the listing is not on flash sale. */}
            <LiveFlashSaleStock
              listingId={listing.id}
              campaign={flashSaleCampaign}
              currency={listing.currency}
            />

            {/* Shopee Vouchers & Perks */}
            <div className="space-y-2.5 text-xs text-gray-600">
              <div className="flex items-center gap-3">
                <span className="w-24 text-gray-500">Mã Giảm Giá:</span>
                <div className="flex flex-wrap gap-1.5">
                  <span className="rounded-xs bg-red-50 px-2 py-0.5 font-semibold text-brand border border-red-200">
                    Giảm ₫50k
                  </span>
                  <span className="rounded-xs bg-red-50 px-2 py-0.5 font-semibold text-brand border border-red-200">
                    Giảm 10%
                  </span>
                  <span className="rounded-xs bg-orange-50 px-2 py-0.5 font-semibold text-brand border border-brand/30">
                    Freeship Xtra
                  </span>
                </div>
              </div>

              <div className="flex items-center gap-3">
                <span className="w-24 text-gray-500">Vận Chuyển:</span>
                <div>
                  <p className="font-medium text-gray-800 flex items-center gap-1">
                    <span>🚚</span>
                    <span>Miễn phí vận chuyển cho đơn hàng từ 0Đ</span>
                  </p>
                  <p className="text-[11px] text-gray-400 mt-0.5">
                    Giao hàng nhanh bởi SPX Express (1-2 ngày)
                  </p>
                </div>
              </div>

              <div className="flex items-center gap-3">
                <span className="w-24 text-gray-500">Bảo Hiểm:</span>
                <span className="text-gray-800 font-medium">
                  Bảo hiểm thiết bị & bảo vệ người mua an toàn 100%
                </span>
              </div>
            </div>

            {/* Variants Selection (if any) */}
            {listing.variants && listing.variants.length > 0 && (
              <div className="flex items-start gap-3 border-t pt-3 text-xs">
                <span className="w-24 text-gray-500 pt-1.5">Phân Loại:</span>
                <div className="flex flex-wrap gap-2">
                  {listing.variants.map((v) => (
                    <button
                      key={v.id || v.name}
                      type="button"
                      className="rounded-xs border border-gray-200 bg-white px-3 py-1.5 text-xs text-gray-800 hover:border-brand hover:text-brand transition focus:border-brand focus:text-brand"
                    >
                      {v.name}
                    </button>
                  ))}
                </div>
              </div>
            )}

            {/* Buy & Add to Cart Actions */}
            <div className="border-t pt-4 space-y-3">
              <AddToCartButton
                listing={listing}
                selectedVariantId={listing.variants?.[0]?.id}
              />

              {/* Wishlist: save this listing into a named collection */}
              <AddToCollectionButton
                listingId={listing.id}
                initialCollections={collections}
                loggedIn={loggedIn}
              />

              {/* Price-drop / back-in-stock alert toggles */}
              <AlertToggle
                listingId={listing.id}
                initial={alertState}
                loggedIn={loggedIn}
              />
            </div>

            {/* Guarantee Assurance */}
            <div className="flex items-center gap-6 border-t pt-3 text-[11px] text-gray-500">
              <span className="flex items-center gap-1 text-brand font-medium">
                <span>🛡️</span>
                <span>Đảm Bảo Hoàn Tiền</span>
              </span>
              <span>7 Ngày Miễn Phí Trả Hàng</span>
              <span>Hàng Chính Hãng 100%</span>
            </div>
          </div>
        </div>
      </div>

      {/* ── Shop Profile Card ── */}
      <div className="rounded-xs bg-white p-5 shadow-2xs flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div className="flex items-center gap-4">
          <div className="grid h-16 w-16 place-items-center rounded-full bg-orange-100 text-2xl font-bold text-brand">
            🛍️
          </div>
          <div>
            <h3 className="font-bold text-sm text-gray-900 flex items-center gap-1.5">
              <span>Official Store Partner</span>
              <span className="rounded-2xs bg-[#d0011b] px-1 py-0.2 text-[9px] font-bold text-white uppercase">
                Mall
              </span>
            </h3>
            <p className="text-xs text-gray-400 mt-0.5">Online 2 phút trước</p>
            <div className="mt-2 flex gap-2">
              <ChatWithSellerButton
                sellerId={listing.sellerId}
                listingId={listing.id}
                loggedIn={loggedIn}
              />
              <Link
                href={`/shop/${listing.sellerId}`}
                className="rounded-xs border border-gray-300 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50 transition"
              >
                Xem Shop
              </Link>
            </div>
          </div>
        </div>

        <div className="grid grid-cols-3 gap-6 text-xs text-gray-500 border-t sm:border-t-0 sm:border-l sm:pl-6 pt-3 sm:pt-0">
          <div>
            <span className="block text-gray-400">Đánh Giá</span>
            <ShopRatingSummary summary={shopSummary} />
          </div>
          <div>
            <span className="block text-gray-400">Tỉ Lệ Phản Hồi</span>
            <strong className="text-brand">99%</strong>
          </div>
          <div>
            <span className="block text-gray-400">Thời Gian Phản Hồi</span>
            <strong className="text-gray-800">Trong vài phút</strong>
          </div>
        </div>
      </div>

      {/* ── Product Description & Details ── */}
      <div className="rounded-xs bg-white p-6 shadow-2xs space-y-4">
        <h2 className="rounded-xs bg-[#fafafa] p-3 text-xs font-bold uppercase tracking-wider text-gray-800">
          CHI TIẾT SẢN PHẨM
        </h2>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-xs text-gray-600 px-3">
          <div className="flex gap-2">
            <span className="w-28 text-gray-400">Danh Mục:</span>
            <span className="text-brand font-medium">
              Shopee &gt; Hàng Chính Hãng
            </span>
          </div>
          <div className="flex gap-2">
            <span className="w-28 text-gray-400">Kho hàng:</span>
            <span className="text-gray-800 font-semibold">
              {listing.stock} sản phẩm
            </span>
          </div>
          <div className="flex gap-2">
            <span className="w-28 text-gray-400">Thương hiệu:</span>
            <span className="text-gray-800 font-semibold">Chính Hãng 100%</span>
          </div>
          <div className="flex gap-2">
            <span className="w-28 text-gray-400">Gửi từ:</span>
            <span className="text-gray-800">TP. Hồ Chí Minh / Hà Nội</span>
          </div>
        </div>

        <h2 className="rounded-xs bg-[#fafafa] p-3 text-xs font-bold uppercase tracking-wider text-gray-800 mt-6">
          MÔ TẢ SẢN PHẨM
        </h2>
        <div className="px-3 text-xs text-gray-700 leading-relaxed whitespace-pre-line">
          {listing.description}
        </div>
      </div>

      {/* ── Customer Reviews & Ratings with Photos, Helpful votes and
             verified-purchase badges — real data from team-engagement. ── */}
      {aiReviewSummary && <AiReviewSummary summary={aiReviewSummary} />}

      <ReviewSection
        listingId={listing.id}
        productTitle={listing.title}
        initialSummary={ratingSummary}
        initialReviews={reviews}
      />

      {/* ── Community Q&A — real buyer questions + seller answers from
             team-engagement (gateway-only). ── */}
      <QASection
        listingId={listing.id}
        loggedIn={loggedIn}
        initialQuestions={questions}
      />

      {/* ── "Gợi ý cho bạn" — similar items seeded with this listing, sourced
             from team-ai (RecommendationService/Recommend) via the gateway.
             Hidden when the service is unavailable. ── */}
      <RecommendationsRow seedListingId={listing.id} />
    </div>
  );
}
