import { notFound, redirect } from "next/navigation";
import type { ReactNode } from "react";

import { getPrincipal } from "@/lib/gateway/session";

import { loadOrderResult } from "./data";

/**
 * Decides the HTTP status: an unknown order answers a real 404 (UI in
 * account/orders/not-found.tsx) before loading.tsx streams its skeleton. The
 * page still renders the 403 and error states itself.
 */
export default async function BuyerOrderLayout({
  children,
  params,
}: { children: ReactNode; params: { id: string } }) {
  if (!getPrincipal()) redirect("/login");
  const res = await loadOrderResult(params.id);
  if (res.kind === "not_found") notFound();
  return children;
}
