"use client";

import { trackEcommerce } from "@/lib/analytics";
import { useEffect, useRef } from "react";

export function ExperimentExposure({
  flag,
  variant,
}: {
  flag: string;
  variant: string | null | undefined;
}) {
  const sent = useRef(false);
  useEffect(() => {
    if (sent.current || !variant) return;
    sent.current = true;
    try {
      trackEcommerce("experiment_exposure", { properties: { flag, variant } });
    } catch {
      // exposure must never break the page
    }
  }, [flag, variant]);
  return null;
}
