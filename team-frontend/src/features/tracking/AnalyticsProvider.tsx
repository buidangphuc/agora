"use client";

import { applyConsent } from "@/lib/analytics/destinations/gtm";
import { type ConsentGroups, readConsent } from "@/lib/consent/consent";
import { type ReactNode, useEffect } from "react";

const CHANGE_EVENT = "agora:consent:change";

export function AnalyticsProvider({ children }: { children: ReactNode }) {
  useEffect(() => {
    // generation bumps on withdrawal so late identity responses are discarded
    let generation = 0;
    let requested = false;

    const pushUserId = (userId: string) => {
      window.dataLayer = window.dataLayer || [];
      (window.dataLayer as unknown[]).push({ user_id: userId });
    };

    const fetchIdentity = () => {
      if (requested) return;
      requested = true;
      const mine = generation;
      fetch("/api/analytics/identity")
        .then(async (res) => {
          if (res.status !== 200) return;
          const body = (await res.json()) as { userId?: unknown };
          if (mine !== generation) return;
          if (typeof body.userId === "string" && body.userId)
            pushUserId(body.userId);
        })
        .catch(() => {});
    };

    const apply = (groups: ConsentGroups) => {
      applyConsent({
        analytics: groups.analytics,
        marketing: groups.marketing,
        personalization: groups.personalization,
      });
      if (groups.analytics) {
        fetchIdentity();
      } else {
        generation++;
        requested = false;
      }
    };
    const onChange = (e: Event) => {
      const detail = (e as CustomEvent<ConsentGroups | undefined>).detail;
      apply(detail ?? readConsent());
    };
    apply(readConsent());
    window.addEventListener(CHANGE_EVENT, onChange);
    return () => {
      generation++;
      window.removeEventListener(CHANGE_EVENT, onChange);
    };
  }, []);

  return <>{children}</>;
}
