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

export const dynamic = "force-dynamic";

/**
 * Home: static blocks first (hero, hubs), then every data block streams in its
 * own Suspense boundary with a same-footprint skeleton, so a slow gateway call
 * never blocks the rest of the page.
 */
export default function HomePage() {
  return (
    <div className="space-y-6">
      <Suspense fallback={null}>
        <LoyaltyBlock />
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
