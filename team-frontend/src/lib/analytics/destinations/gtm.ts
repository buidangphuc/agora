/**
 * Google Tag Manager integration with Consent Mode v2 (basic mode).
 * GTM is injected only after analytics consent and only when
 * NEXT_PUBLIC_GTM_ID is non-blank. State is derived from window.dataLayer
 * and the DOM, so applyConsent is idempotent.
 */

import type { ConsentGroups } from "@/lib/consent/consent";

const SCRIPT_SELECTOR = 'script[src*="googletagmanager.com/gtm.js"]';

const DENIED_DEFAULT = {
  analytics_storage: "denied",
  ad_storage: "denied",
  ad_user_data: "denied",
  ad_personalization: "denied",
  personalization_storage: "denied",
  functionality_storage: "denied",
  security_storage: "denied",
};

type Signals = Record<string, string>;

function consentEntries(layer: unknown[]): [string, Signals][] {
  const out: [string, Signals][] = [];
  for (const e of layer) {
    if (e && typeof e === "object" && "length" in e) {
      const a = Array.from(e as ArrayLike<unknown>);
      if (a[0] === "consent") out.push([a[1] as string, a[2] as Signals]);
    }
  }
  return out;
}

export function applyConsent(groups: ConsentGroups): void {
  if (typeof window === "undefined") return;

  window.dataLayer = window.dataLayer || [];
  const layer = window.dataLayer as unknown[];
  // gtag.js/GTM only process Arguments objects as commands, not arrays.
  function gtag(..._args: unknown[]) {
    // biome-ignore lint/style/noArguments: GTM requires Arguments objects
    layer.push(arguments);
  }

  let entries = consentEntries(layer);
  if (!entries.some(([k]) => k === "default")) {
    gtag("consent", "default", { ...DENIED_DEFAULT });
    entries = consentEntries(layer);
  }

  const grant = (b: boolean) => (b ? "granted" : "denied");
  const lastUpdate = [...entries].reverse().find(([k]) => k === "update")?.[1];

  if (groups.analytics) {
    const signals: Signals = {
      analytics_storage: "granted",
      ad_storage: grant(groups.marketing),
      ad_user_data: grant(groups.marketing),
      ad_personalization: grant(groups.marketing),
      personalization_storage: grant(groups.personalization),
    };
    const same =
      lastUpdate &&
      Object.keys(signals).every((k) => lastUpdate[k] === signals[k]);
    if (!same) gtag("consent", "update", signals);

    if (
      process.env.NEXT_PUBLIC_APP_ENV === "staging" &&
      !layer.some(
        (e) =>
          !!e &&
          typeof e === "object" &&
          (e as { debug_mode?: unknown }).debug_mode === true,
      )
    ) {
      layer.push({ debug_mode: true });
    }

    const gtmId = process.env.NEXT_PUBLIC_GTM_ID?.trim();
    if (gtmId && !document.querySelector(SCRIPT_SELECTOR)) {
      layer.push({ "gtm.start": new Date().getTime(), event: "gtm.js" });
      const script = document.createElement("script");
      script.async = true;
      script.src = `https://www.googletagmanager.com/gtm.js?id=${encodeURIComponent(gtmId)}`;
      document.head.appendChild(script);
    }
  } else if (lastUpdate && lastUpdate.analytics_storage === "granted") {
    gtag("consent", "update", {
      analytics_storage: "denied",
      ad_storage: "denied",
      ad_user_data: "denied",
      ad_personalization: "denied",
      personalization_storage: "denied",
    });
  }
}
