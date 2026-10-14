import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { notFound } from "next/navigation";
import type { ReactNode } from "react";

import { getPrincipal } from "@/lib/gateway/session";

import { loadOrderResult } from "./data";

/**
 * Decides the HTTP status: an unknown order answers a real 404 (UI in
 * account/orders/not-found.tsx) before loading.tsx streams its skeleton. The
 * page still renders the 403 and error states itself.
 */
export default async function BuyerOrderLayout(props: {
  children: ReactNode;
  params: Promise<{ id: string }>;
}) {
  const params = await props.params;

  const { children } = props;

  if (!getPrincipal())
    redirectGuestToLogin(`/account/orders/${encodeURIComponent(params.id)}`);
  const res = await loadOrderResult(params.id);
  if (res.kind === "not_found") notFound();
  return children;
}
