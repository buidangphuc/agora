"use client";

import { useEffect, useRef } from "react";

import { type EcommerceItem, trackEcommerce } from "@/lib/analytics";

/**
 * Fires `begin_checkout` once per checkout entry. It is mounted by the checkout
 * page, which stays mounted across step navigation, so changing step never
 * re-fires it (the ref also guards React strict-mode double effects).
 */
export function BeginCheckoutBeacon({
  value,
  items,
}: {
  value: number;
  items: EcommerceItem[];
}) {
  const fired = useRef(false);
  useEffect(() => {
    if (fired.current) return;
    fired.current = true;
    trackEcommerce("begin_checkout", { currency: "VND", value, items });
  }, [value, items]);
  return null;
}
