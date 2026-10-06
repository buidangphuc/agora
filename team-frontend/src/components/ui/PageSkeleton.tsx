/**
 * Generic page skeleton (Ant Design Skeleton pattern): reserves the page
 * container so content swaps in without layout shift. Routes without a bespoke
 * skeleton re-export it from their own segment `loading.tsx`; it must NOT live
 * at the app root, where its Suspense boundary would stream a 200 status before
 * any page can call notFound().
 */
const TILES = [
  "t1",
  "t2",
  "t3",
  "t4",
  "t5",
  "t6",
  "t7",
  "t8",
  "t9",
  "t10",
  "t11",
  "t12",
];

export function PageSkeleton() {
  return (
    <div
      className="space-y-4"
      aria-busy="true"
      aria-live="polite"
      data-testid="page-skeleton"
    >
      <div className="h-8 w-1/3 animate-pulse rounded-lg bg-neutral-200" />
      <div className="aspect-2/1 w-full animate-pulse rounded-2xl bg-neutral-200" />
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4 lg:grid-cols-6">
        {TILES.map((id) => (
          <div
            key={id}
            className="aspect-square animate-pulse rounded-xl bg-neutral-200"
          />
        ))}
      </div>
    </div>
  );
}
