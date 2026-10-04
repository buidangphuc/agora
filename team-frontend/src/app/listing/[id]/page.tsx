import { notFound } from "next/navigation";
import { Suspense } from "react";

import { AlertToggle } from "@/components/alerts/AlertToggle";
import { Breadcrumb } from "@/components/ui/Breadcrumb";
import { Card } from "@/components/ui/Card";
import { Rate } from "@/components/ui/Rate";
import { BuyBar } from "@/features/cart/BuyBar";
import { PurchaseProvider } from "@/features/cart/PurchaseContext";
import { PurchasePanel } from "@/features/cart/PurchasePanel";
import { ChatWithSellerButton } from "@/features/chat/ChatWithSellerButton";
import { AddToCollectionButton } from "@/features/engagement/AddToCollectionButton";
import { FavoriteButton } from "@/features/engagement/FavoriteButton";
import { ImageGallery } from "@/features/listing/ImageGallery";
import { LiveFlashSaleStock } from "@/features/listing/LiveFlashSaleStock";
import { PdpAnchorNav } from "@/features/listing/PdpAnchorNav";
import { SelectedPrice } from "@/features/listing/SelectedPrice";
import { ShareButton } from "@/features/listing/ShareButton";
import { SpecsSection } from "@/features/listing/SpecsSection";
import { VariantSelector } from "@/features/listing/VariantSelector";
import { parsePdpParams, resolveVariant } from "@/features/listing/pdp";
import { QASection } from "@/features/listing/qa/QASection";
import { RecommendationsRow } from "@/features/recommendations/RecommendationsRow";
import { RecommendationsSkeleton } from "@/features/recommendations/RecommendationsSkeleton";
import { ReviewSection } from "@/features/review/ReviewSection";
import { ShopHeaderCard } from "@/features/shop/ShopHeaderCard";
import { TrackView } from "@/features/tracking/TrackView";
import { listCollections, recordView } from "@/lib/gateway/engagement";
import { getCategory, getListing, getStorefront } from "@/lib/gateway/listings";
import { AlertType, listAlertSubscriptions } from "@/lib/gateway/notification";
import { getActiveFlashSale } from "@/lib/gateway/promotion";
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
  searchParams,
}: {
  params: { id: string };
  searchParams: Record<string, string | string[] | undefined>;
}) {
  // getListing returns null only for gRPC NotFound; any other error throws and
  // reaches error.tsx.
  const listing = await getListing(params.id);
  if (!listing) notFound();

  const query = parsePdpParams(searchParams);
  const resolved = resolveVariant(listing, query.variant);

  const me = getPrincipal();
  const loggedIn = me !== null;

  // Reviews are fetched once and shared by the AI summary and the review list;
  // both await it inside their own Suspense boundaries, so the page does not
  // wait for them (nor for team-ai).
  const reviewsPromise = listReviews(params.id);

  // Critical set only: everything the header needs for its first paint.
  const [
    flashSale,
    ratingSummary,
    shopSummary,
    storefront,
    category,
    collections,
    alertSubs,
  ] = await Promise.all([
    getActiveFlashSale(params.id),
    getListingRatingSummary(params.id),
    getShopRatingSummary(listing.sellerId),
    getStorefront(listing.sellerId),
    listing.categoryId
      ? getCategory(listing.categoryId)
      : Promise.resolve(null),
    loggedIn ? listCollections() : Promise.resolve([]),
    loggedIn ? listAlertSubscriptions() : Promise.resolve([]),
    // Best-effort: record this view so it shows up in "Vừa xem" (recently
    // viewed). Never throws; its result is ignored.
    recordView(params.id),
  ]);

  const flashSaleCampaign =
    flashSale.active && flashSale.campaign ? flashSale.campaign : null;
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
      : listing.imageUrl
        ? [listing.imageUrl]
        : [];

  const breadcrumb = [
    { label: "Trang chủ", href: "/" },
    ...(category
      ? [{ label: category.name, href: `/search?category=${category.id}` }]
      : []),
    { label: listing.title },
  ];

  const rated = ratingSummary.reviewCount > 0;

  return (
    <PurchaseProvider
      listing={{
        id: listing.id,
        title: listing.title,
        categoryId: listing.categoryId,
        price: listing.price,
        stock: listing.stock,
        currency: listing.currency,
        variants: listing.variants,
        flashSale: flashSaleCampaign
          ? {
              variantId: flashSaleCampaign.variantId,
              salePrice: flashSaleCampaign.salePrice,
            }
          : null,
      }}
      initialVariantId={resolved.id}
    >
      <div className="space-y-4">
        {/* Fire a best-effort VIEW tracking beacon on PDP mount. */}
        <TrackView listingId={listing.id} path={`/listing/${listing.id}`} />

        <Breadcrumb items={breadcrumb} />

        {/* ── Header card: gallery + info column ── */}
        <Card className="p-6">
          <div className="grid grid-cols-1 gap-8 lg:grid-cols-12">
            <div className="lg:col-span-5">
              <ImageGallery
                images={images}
                alt={listing.title}
                overlay={<FavoriteButton id={listing.id} initial={false} />}
              />
            </div>

            <div className="space-y-4 lg:col-span-7">
              <h1 className="text-xl font-semibold leading-snug text-text-primary">
                {listing.title}
              </h1>

              <div
                data-testid="pdp-rating"
                className="flex flex-wrap items-center gap-2 border-b border-border-subtle pb-3 text-sm"
              >
                {rated ? (
                  <>
                    <Rate
                      readOnly
                      size="sm"
                      value={ratingSummary.averageRating}
                    />
                    <span className="font-semibold text-text-primary">
                      {ratingSummary.averageRating.toFixed(1)}
                    </span>
                    <span className="text-text-secondary">
                      ({ratingSummary.reviewCount} đánh giá)
                    </span>
                  </>
                ) : (
                  <span className="text-text-secondary">Chưa có đánh giá</span>
                )}
              </div>

              {/* Price: the strike-through exists only for a real flash-sale
                  price; the page never computes a compare-at price. */}
              <div
                data-testid="pdp-price"
                className="rounded-xl bg-surface-muted p-4"
              >
                <SelectedPrice />
              </div>

              {/* Live flash sale (team-promotion). Renders nothing when the
                  listing is not on flash sale. */}
              <LiveFlashSaleStock
                listingId={listing.id}
                campaign={flashSaleCampaign}
                currency={listing.currency}
              />

              <dl className="space-y-2 text-sm">
                <div className="flex gap-3">
                  <dt className="w-24 shrink-0 text-text-secondary">
                    Vận chuyển
                  </dt>
                  <dd className="text-text-primary">
                    Miễn phí vận chuyển cho đơn hàng từ 0Đ. Giao hàng nhanh bởi
                    SPX Express (1-2 ngày)
                  </dd>
                </div>
                <div className="flex gap-3">
                  <dt className="w-24 shrink-0 text-text-secondary">
                    Bảo hiểm
                  </dt>
                  <dd className="text-text-primary">
                    Bảo hiểm thiết bị và bảo vệ người mua
                  </dd>
                </div>
              </dl>

              <VariantSelector />

              <div className="border-t border-border-subtle pt-4">
                <PurchasePanel />
              </div>

              <div className="flex flex-wrap items-center gap-3 border-t border-border-subtle pt-4">
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
                {/* Share short link (team-sharing via gateway) */}
                <ShareButton id={listing.id} />
              </div>

              <p className="flex flex-wrap gap-x-6 gap-y-1 border-t border-border-subtle pt-3 text-sm text-text-secondary">
                <span>Đảm bảo hoàn tiền</span>
                <span>7 ngày miễn phí trả hàng</span>
              </p>
            </div>
          </div>
        </Card>

        <ShopHeaderCard
          variant="compact"
          sellerId={listing.sellerId}
          storefront={storefront}
          summary={shopSummary}
          actions={
            <ChatWithSellerButton
              sellerId={listing.sellerId}
              listingId={listing.id}
              loggedIn={loggedIn}
            />
          }
        />

        {/* ── Body: anchor nav + three stacked, always-visible sections ── */}
        <div className="space-y-4">
          <PdpAnchorNav />
          <SpecsSection
            categoryName={category?.name}
            hasSku={listing.variants.some((v) => v.sku !== "")}
            description={listing.description}
          />
          <ReviewSection
            listingId={listing.id}
            productTitle={listing.title}
            summary={ratingSummary}
            reviewsPromise={reviewsPromise}
            rating={query.rating}
            rpage={query.rpage}
            variant={query.variant}
            loggedIn={loggedIn}
          />
          <QASection listingId={listing.id} loggedIn={loggedIn} />
        </div>

        {/* "Gợi ý cho bạn": similar items seeded with this listing, from
            team-ai via the gateway. Hidden when the service is unavailable. */}
        <Suspense fallback={<RecommendationsSkeleton />}>
          <RecommendationsRow seedListingId={listing.id} />
        </Suspense>
      </div>

      <BuyBar />
    </PurchaseProvider>
  );
}
