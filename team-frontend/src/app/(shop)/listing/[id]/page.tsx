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
import { ReportListingButton } from "@/features/listing/ReportListingButton";
import { SelectedPrice } from "@/features/listing/SelectedPrice";
import { ShareButton } from "@/features/listing/ShareButton";
import {
  ShippingInfo,
  ShippingInfoFallback,
} from "@/features/listing/ShippingInfo";
import { SpecsSection } from "@/features/listing/SpecsSection";
import { VariantSelector } from "@/features/listing/VariantSelector";
import { parsePdpParams, resolveVariant } from "@/features/listing/pdp";
import { QASection } from "@/features/listing/qa/QASection";
import { RecommendationsRow } from "@/features/recommendations/RecommendationsRow";
import { RecommendationsSkeleton } from "@/features/recommendations/RecommendationsSkeleton";
import { ReviewSection } from "@/features/review/ReviewSection";
import { ShopHeaderCard } from "@/features/shop/ShopHeaderCard";
import { TrackView } from "@/features/tracking/TrackView";
import {
  isFavorite,
  listCollections,
  recordView,
} from "@/lib/gateway/engagement";
import { getCategory, getStorefront } from "@/lib/gateway/listings";
import { AlertType, listAlertSubscriptions } from "@/lib/gateway/notification";
import { getActiveFlashSale } from "@/lib/gateway/promotion";
import {
  getListingRatingSummary,
  getShopRatingSummary,
  listReviews,
  listReviewsPage,
} from "@/lib/gateway/reviews";
import { getPrincipal } from "@/lib/gateway/session";
import { getImageUrl } from "@/lib/media";

import { loadListing } from "./data";

export const dynamic = "force-dynamic";

export default async function ProductDetailPage(props: {
  params: Promise<{ id: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const searchParams = await props.searchParams;
  const params = await props.params;
  // getListing returns null only for gRPC NotFound; any other error throws and
  // reaches error.tsx.
  const listing = await loadListing(params.id);
  if (!listing) notFound();

  const query = parsePdpParams(searchParams);
  const resolved = resolveVariant(listing, query.variant);

  const me = getPrincipal();
  const loggedIn = me !== null;
  const isAdmin = me?.scopes.includes("admin") ?? false;

  // The AI summary samples the newest reviews; the review list asks the server
  // for just the `?rpage=` page (and `?rating=` filter). Both are awaited inside
  // their own Suspense boundaries, so the page does not wait for them (nor for
  // team-ai).
  const reviewsPromise = listReviews(params.id);
  const reviewsPagePromise = listReviewsPage(params.id, {
    rating: query.rating,
    page: query.rpage,
  });

  // Critical set only: everything the header needs for its first paint.
  const [
    flashSale,
    ratingSummary,
    shopSummary,
    storefront,
    category,
    collections,
    alertSubs,
    favorited,
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
    // Saved-favorite state, so the heart is filled after a reload.
    loggedIn ? isFavorite(params.id) : Promise.resolve(false),
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
      <div className="space-y-6 md:space-y-8">
        <TrackView listingId={listing.id} path={`/listing/${listing.id}`} />

        <Breadcrumb items={breadcrumb} />

        <div className="grid grid-cols-1 gap-6 md:grid-cols-2 md:gap-8 lg:grid-cols-12 lg:gap-10">
          <div className="min-w-0 md:sticky md:top-32 md:self-start lg:col-span-6">
            <ImageGallery
              images={images}
              alt={listing.title}
              overlay={<FavoriteButton id={listing.id} initial={favorited} />}
            />
          </div>

          <div className="min-w-0 lg:col-span-6">
            <Card padded>
              <div className="flex flex-col gap-6">
                <div className="flex flex-col gap-3">
                  <h1 className="break-words text-2xl font-semibold leading-tight tracking-tight text-text-primary md:text-3xl">
                    {listing.title}
                  </h1>

                  <div
                    data-testid="pdp-rating"
                    className="flex flex-wrap items-center gap-2 text-sm"
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
                        <a
                          href="#reviews"
                          className="text-text-secondary underline-offset-4 hover:text-text-primary hover:underline"
                        >
                          ({ratingSummary.reviewCount} đánh giá)
                        </a>
                      </>
                    ) : (
                      <span className="text-text-secondary">
                        Chưa có đánh giá
                      </span>
                    )}
                  </div>
                </div>

                <div
                  data-testid="pdp-price"
                  className="rounded-2xl bg-surface-muted px-5 py-4"
                >
                  <SelectedPrice />
                </div>

                <LiveFlashSaleStock
                  listingId={listing.id}
                  campaign={flashSaleCampaign}
                  currency={listing.currency}
                />

                {me ? (
                  <Suspense fallback={<ShippingInfoFallback />}>
                    <ShippingInfo signedIn />
                  </Suspense>
                ) : (
                  <ShippingInfoFallback />
                )}

                <VariantSelector />

                <div className="border-t border-border-subtle pt-6">
                  <PurchasePanel />
                </div>

                <div className="flex flex-col gap-4 border-t border-border-subtle pt-6">
                  <div className="flex flex-wrap items-center gap-3">
                    <AddToCollectionButton
                      listingId={listing.id}
                      initialCollections={collections}
                      loggedIn={loggedIn}
                    />
                    <ShareButton id={listing.id} />
                  </div>
                  <AlertToggle
                    listingId={listing.id}
                    initial={alertState}
                    loggedIn={loggedIn}
                  />
                </div>

                <ul className="flex flex-wrap gap-2 text-sm text-text-secondary">
                  <li className="rounded-full bg-surface-muted px-3 py-1.5">
                    Đảm bảo hoàn tiền
                  </li>
                  <li className="rounded-full bg-surface-muted px-3 py-1.5">
                    7 ngày miễn phí trả hàng
                  </li>
                </ul>

                {me?.id !== listing.sellerId && (
                  <div className="border-t border-border-subtle pt-4">
                    <ReportListingButton
                      listingId={listing.id}
                      loggedIn={loggedIn}
                    />
                  </div>
                )}
              </div>
            </Card>
          </div>
        </div>

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

        <div className="space-y-6 md:space-y-8">
          <PdpAnchorNav />
          <SpecsSection
            categoryName={category?.name}
            hasSku={listing.variants.some((v) => v.sku !== "")}
            description={listing.description}
          />
          <ReviewSection
            listingId={listing.id}
            summary={ratingSummary}
            reviewsPromise={reviewsPromise}
            pagePromise={reviewsPagePromise}
            rating={query.rating}
            variant={query.variant}
            isAdmin={isAdmin}
          />
          <QASection listingId={listing.id} loggedIn={loggedIn} />
        </div>

        <Suspense fallback={<RecommendationsSkeleton />}>
          <RecommendationsRow seedListingId={listing.id} />
        </Suspense>
      </div>

      <BuyBar />
    </PurchaseProvider>
  );
}
