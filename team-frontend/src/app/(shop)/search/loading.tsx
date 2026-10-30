import {
  FilterSkeleton,
  ResultsSkeleton,
  SearchHeaderSkeleton,
} from "@/features/search/SearchSkeletons";

/** Route skeleton for `/search`: the same header, filter column and 24-card grid. */
export default function SearchLoading() {
  return (
    <section className="space-y-6 py-4 md:space-y-8 md:py-8" aria-busy="true">
      <SearchHeaderSkeleton />
      <div className="flex flex-col gap-6 lg:flex-row lg:gap-8">
        <div className="w-full space-y-4 lg:w-72 lg:shrink-0">
          <FilterSkeleton />
        </div>
        <div className="min-w-0 flex-1">
          <ResultsSkeleton />
        </div>
      </div>
    </section>
  );
}
