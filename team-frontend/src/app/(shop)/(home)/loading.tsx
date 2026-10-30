import {
  CategoryGridSkeleton,
  FeedSkeleton,
  HeroSkeleton,
  ServiceHubsSkeleton,
} from "@/features/home/HomeSkeletons";

/** Route-level skeleton for `/`: hero, hubs, category grid and the feed grid. */
export default function HomeLoading() {
  return (
    <div className="flex flex-col gap-8 sm:gap-12" aria-busy="true">
      <HeroSkeleton />
      <ServiceHubsSkeleton />
      <CategoryGridSkeleton />
      <FeedSkeleton />
    </div>
  );
}
