import { notFound } from "next/navigation";
import type { ReactNode } from "react";

import { hasScope } from "@/lib/gateway/session";

import { loadListing } from "./data";

/**
 * Decides the HTTP status: an unknown listing answers a real 404 (UI in
 * seller/not-found.tsx) before loading.tsx streams its skeleton. The page keeps
 * the sign-in redirect and the ownership check.
 */
export default async function SellerEditLayout({
  children,
  params,
}: { children: ReactNode; params: { id: string } }) {
  if (!hasScope("listing.write")) return children;
  if (!(await loadListing(params.id))) notFound();
  return children;
}
