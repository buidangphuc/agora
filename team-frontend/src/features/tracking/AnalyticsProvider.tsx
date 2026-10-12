"use client";

import { applyConsent } from "@/lib/analytics/destinations/gtm";
import { type ConsentGroups, readConsent } from "@/lib/consent/consent";
import { type ReactNode, useEffect } from "react";

const CHANGE_EVENT = "agora:consent:change";

export function AnalyticsProvider({ children }: { children: ReactNode }) {
  useEffect(() => {
    const apply = (groups: ConsentGroups) =>
      applyConsent({
        analytics: groups.analytics,
        marketing: groups.marketing,
        personalization: groups.personalization,
      });
    const onChange = (e: Event) => {
      const detail = (e as CustomEvent<ConsentGroups | undefined>).detail;
      apply(detail ?? readConsent());
    };
    apply(readConsent());
    window.addEventListener(CHANGE_EVENT, onChange);
    return () => window.removeEventListener(CHANGE_EVENT, onChange);
  }, []);

  return <>{children}</>;
}
