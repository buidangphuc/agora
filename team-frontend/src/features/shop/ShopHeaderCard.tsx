import Link from "next/link";
import type { ReactNode } from "react";

import { Avatar } from "@/components/ui/Avatar";
import { Card } from "@/components/ui/Card";
import { Image } from "@/components/ui/Image";
import { Statistic } from "@/components/ui/Statistic";
import { ShopRatingSummary } from "@/features/review/ShopRatingSummary";
import type { ViewStorefront } from "@/lib/gateway/listings";
import type { ViewShopRatingSummary } from "@/lib/gateway/reviews";
import { shopLabel } from "@/lib/gateway/shops";

/**
 * One shop header for the product page (`compact`) and the storefront
 * (`hero`: banner, tagline and a stat row). Server component; the follow and
 * chat buttons are client leaves passed through `actions`. Only real data is
 * shown: the shop display name (fallback "Shop #<6 chars>" for an empty name)
 * and the shop rating summary. No response rate, tenure or "Online" text.
 */
export function ShopHeaderCard({
  sellerId,
  storefront,
  summary,
  productCount,
  actions,
  variant = "compact",
}: {
  sellerId: string;
  storefront: ViewStorefront | null;
  summary: ViewShopRatingSummary;
  /** Hero only: number of products shown on the storefront. */
  productCount?: number;
  actions?: ReactNode;
  variant?: "compact" | "hero";
}) {
  const name = shopLabel(sellerId, storefront?.displayName);

  if (variant === "compact") {
    return (
      <Card data-testid="shop-header-card" className="p-5">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex min-w-0 items-center gap-4">
            <Avatar name={name} size="lg" />
            <div className="min-w-0 space-y-1.5">
              <h3
                data-testid="shop-name"
                className="truncate text-base font-semibold text-text-primary"
              >
                {name}
              </h3>
              <ShopRatingSummary summary={summary} />
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {actions}
            <Link
              href={`/shop/${sellerId}`}
              className="inline-flex items-center justify-center rounded-lg border border-border-strong bg-surface-card px-3 py-1.5 text-xs font-medium text-text-primary shadow-sm transition duration-150 hover:bg-surface-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2"
            >
              Xem Shop
            </Link>
          </div>
        </div>
      </Card>
    );
  }

  const rated = summary.reviewCount > 0;
  return (
    <Card data-testid="shop-header-card">
      {storefront?.bannerUrl && (
        <Image
          src={storefront.bannerUrl}
          alt="Ảnh bìa gian hàng"
          aspect="2/1"
          loading="eager"
          className="rounded-none"
        />
      )}
      {storefront?.tagline && (
        <p className="border-b border-border-subtle px-5 py-3 text-sm font-medium text-text-primary">
          {storefront.tagline}
        </p>
      )}
      <div className="grid grid-cols-1 gap-6 p-6 md:grid-cols-12 md:items-center">
        <div className="flex min-w-0 items-center gap-4 md:col-span-5">
          <Avatar name={name} size="xl" />
          <div className="min-w-0 space-y-3">
            <h1
              data-testid="shop-name"
              className="truncate text-xl font-semibold text-text-primary"
            >
              {name}
            </h1>
            <div className="flex flex-wrap items-center gap-2">{actions}</div>
          </div>
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 md:col-span-7">
          {productCount !== undefined && (
            <Statistic title="Sản phẩm" value={productCount} />
          )}
          <div data-testid="shop-rating-summary">
            {rated ? (
              <Statistic
                title="Đánh giá"
                value={summary.averageRating.toFixed(1)}
                suffix={`/ 5.0 (${summary.reviewCount} đánh giá)`}
              />
            ) : (
              <Statistic title="Đánh giá" value="Chưa có đánh giá" />
            )}
          </div>
        </div>
      </div>
    </Card>
  );
}
