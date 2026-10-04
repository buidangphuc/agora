import {
  CategoryGridSkeleton,
  FeedSkeleton,
  HeroSkeleton,
  ServiceHubsSkeleton,
} from "@/features/home/HomeSkeletons";

/** Route-level skeleton for `/`: hero, hubs, category grid and the feed grid. */
export default function HomeLoading() {
  return (
    <div className="space-y-6" aria-busy="true">
      <HeroSkeleton />
      <ServiceHubsSkeleton />
      <CategoryGridSkeleton />
      <FeedSkeleton />
    </div>
  );
}
