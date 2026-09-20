"use client";

import { useEffect, type ReactNode } from "react";
import { initGTM } from "@/lib/analytics/destinations/gtm";

export function AnalyticsProvider({ children }: { children: ReactNode }) {
  useEffect(() => {
    // Initialize dataLayer and conditionally inject GTM
    initGTM();
  }, []);

  return <>{children}</>;
}
