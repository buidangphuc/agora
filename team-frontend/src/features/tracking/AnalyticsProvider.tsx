"use client";

import { initGTM } from "@/lib/analytics/destinations/gtm";
import { type ReactNode, useEffect } from "react";

export function AnalyticsProvider({ children }: { children: ReactNode }) {
  useEffect(() => {
    // Initialize dataLayer and conditionally inject GTM
    initGTM();
  }, []);

  return <>{children}</>;
}
