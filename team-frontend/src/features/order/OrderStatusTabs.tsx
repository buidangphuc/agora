import React from "react";

import { Tabs } from "@/components/ui/Tabs";
import { ORDER_STATUS_TABS, type OrderStatusKey } from "./paginateOrders";

/** URL of the list for a tab; the page always restarts at 1. */
export function ordersHref(status: OrderStatusKey, page = 1): string {
  const params = new URLSearchParams();
  if (status !== "all") params.set("status", status);
  if (page > 1) params.set("page", String(page));
  const qs = params.toString();
  return qs ? `/account/orders?${qs}` : "/account/orders";
}

/**
 * Status tabs of the buyer order list. The active tab is the `?status=` value:
 * every tab is a link (the core Tabs link variant), so switching resets `page`,
 * survives reload and back/forward, and needs no client JavaScript. The tab bar
 * scrolls horizontally inside its own container on narrow screens.
 */
export function OrderStatusTabs({
  active,
  counts,
}: {
  active: OrderStatusKey;
  counts: Record<OrderStatusKey, number>;
}) {
  return (
    <Tabs
      activeId={active}
      hrefFor={(id) => ordersHref(id as OrderStatusKey)}
      items={ORDER_STATUS_TABS.map((t) => ({
        id: t.id,
        label: t.label,
        badge: counts[t.id],
      }))}
    />
  );
}
