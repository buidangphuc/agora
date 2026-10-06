import { notFound, redirect } from "next/navigation";
import type { ReactNode } from "react";

import { getPrincipal } from "@/lib/gateway/session";

import { loadOrder } from "./data";

/**
 * Decides the HTTP status: an unknown order answers a real 404 (UI in
 * checkout/pay/not-found.tsx) before loading.tsx streams its skeleton.
 */
export default async function PayLayout({
  children,
  params,
}: { children: ReactNode; params: { id: string } }) {
  if (!getPrincipal()) redirect("/login");
  if (!(await loadOrder(params.id))) notFound();
  return children;
}
