import { notFound } from "next/navigation";
import type { ReactNode } from "react";

import { getPrincipal, hasScope } from "@/lib/gateway/session";

import { isOwnedBy, loadSellerOrder } from "./data";

/**
 * Decides the HTTP status: an unknown (or not-yours) order answers a real 404
 * (UI in seller/not-found.tsx) before loading.tsx streams its skeleton. The
 * seller layout owns the sign-in and scope gates, so skip the lookup without them.
 */
export default async function SellerOrderLayout({
  children,
  params,
}: { children: ReactNode; params: { id: string } }) {
  if (!hasScope("listing.write")) return children;
  const order = await loadSellerOrder(params.id);
  if (!order || !isOwnedBy(order, getPrincipal())) notFound();
  return children;
}
