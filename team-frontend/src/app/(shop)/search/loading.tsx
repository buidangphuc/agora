import {
  FilterSkeleton,
  ResultsSkeleton,
  SearchHeaderSkeleton,
} from "@/features/search/SearchSkeletons";

/** Route skeleton for `/search`: the same header, filter column and 24-card grid. */
export default function SearchLoading() {
  return (
    <section className="space-y-4 py-2" aria-busy="true">
      <SearchHeaderSkeleton />
      <div className="flex flex-col gap-6 lg:flex-row">
        <div className="w-full space-y-4 lg:w-64 lg:shrink-0">
          <FilterSkeleton />
        </div>
        <div className="min-w-0 flex-1">
          <ResultsSkeleton />
        </div>
      </div>
    </section>
  );
}
