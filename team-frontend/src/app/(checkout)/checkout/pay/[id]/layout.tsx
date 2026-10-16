import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { notFound } from "next/navigation";
import type { ReactNode } from "react";

import { getPrincipal } from "@/lib/gateway/session";

import { loadOrder } from "./data";

/**
 * Decides the HTTP status: an unknown order answers a real 404 (UI in
 * checkout/pay/not-found.tsx) before loading.tsx streams its skeleton.
 */
export default async function PayLayout(props: {
  children: ReactNode;
  params: Promise<{ id: string }>;
}) {
  const params = await props.params;

  const { children } = props;

  if (!getPrincipal())
    redirectGuestToLogin(`/checkout/pay/${encodeURIComponent(params.id)}`);
  if (!(await loadOrder(params.id))) notFound();
  return children;
}
