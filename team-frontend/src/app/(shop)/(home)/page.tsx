import type { Metadata } from "next";
import { Suspense } from "react";

import { AssistantBlock } from "@/features/home/AssistantBlock";
import { CategoryGridBlock } from "@/features/home/CategoryGridBlock";
import { FeedBlock } from "@/features/home/FeedBlock";
import { FlashSaleBlock } from "@/features/home/FlashSaleBlock";
import { Hero } from "@/features/home/Hero";
import {
  CategoryGridSkeleton,
  FeedSkeleton,
  RowSkeleton,
} from "@/features/home/HomeSkeletons";
import { LoyaltyBlock } from "@/features/home/LoyaltyBlock";
import { RecentlyViewedRow } from "@/features/home/RecentlyViewedRow";
import { ServiceHubs } from "@/features/home/ServiceHubs";
import { RecommendationsRow } from "@/features/recommendations/RecommendationsRow";
import { WelcomePanel } from "@/features/welcome/WelcomePanel";
import { pageMetadata } from "@/lib/seo";

export const dynamic = "force-dynamic";

export function generateMetadata(): Metadata {
  const md = pageMetadata({
    title: "Agora | Sàn thương mại điện tử",
    description:
      "Agora là sàn thương mại điện tử: mua sắm sản phẩm từ nhiều người bán, giá tốt, giao hàng nhanh.",
    path: "/",
  });
  return { ...md, title: { absolute: "Agora | Sàn thương mại điện tử" } };
}

/**
 * Home: static blocks first (hero, hubs), then every data block streams in its
 * own Suspense boundary with a same-footprint skeleton, so a slow gateway call
 * never blocks the rest of the page.
 */
export default function HomePage() {
  return (
    <div className="flex flex-col gap-8 sm:gap-12">
      <Suspense fallback={null}>
        <LoyaltyBlock />
      </Suspense>

      <Suspense fallback={null}>
        <WelcomePanel />
      </Suspense>

      <Hero />
      <ServiceHubs />

      <Suspense fallback={<CategoryGridSkeleton />}>
        <CategoryGridBlock />
      </Suspense>

      <Suspense fallback={null}>
        <FlashSaleBlock />
      </Suspense>

      <Suspense fallback={<FeedSkeleton />}>
        <FeedBlock />
      </Suspense>

      <Suspense fallback={<RowSkeleton />}>
        <RecentlyViewedRow />
      </Suspense>

      <Suspense fallback={<RowSkeleton />}>
        <RecommendationsRow />
      </Suspense>

      <Suspense fallback={null}>
        <AssistantBlock />
      </Suspense>
    </div>
  );
}
