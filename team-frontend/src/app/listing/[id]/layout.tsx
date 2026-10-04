import { notFound } from "next/navigation";
import type { ReactNode } from "react";

import { loadListing } from "./data";

/**
 * Decides the HTTP status. A layout renders outside this segment's loading.tsx
 * Suspense boundary, so an unknown id answers a real 404 before the skeleton
 * streams; the skeleton still covers client navigations. The 404 UI is
 * listing/not-found.tsx (a segment's not-found.tsx never catches its own layout).
 */
export default async function ListingLayout({
  children,
  params,
}: { children: ReactNode; params: { id: string } }) {
  if (!(await loadListing(params.id))) notFound();
  return children;
}
