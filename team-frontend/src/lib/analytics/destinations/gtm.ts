/**
 * Google Tag Manager integration with Consent Mode v2 (basic mode).
 * GTM is injected only after analytics consent and only when
 * NEXT_PUBLIC_GTM_ID is non-blank. State is derived from window.dataLayer
 * and the DOM, so applyConsent is idempotent.
 */

import type { ConsentGroups } from "@/lib/consent/consent";
import { scrubReferrer } from "../scrub";

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

/** True for /admin and /admin/... (segment exact, case-insensitive). */
function isAdminPath(pathname: string): boolean {
  return /^\/admin(\/|$)/i.test(pathname);
}

/** page_location for analytics: admin paths lose all query+hash; others lose returnUrl. */
export function pageLocationFor(href: string): string {
  const loc = new URL(href);
  if (isAdminPath(loc.pathname)) {
    loc.search = "";
    loc.hash = "";
    return loc.toString();
  }
  if (loc.searchParams.has("returnUrl")) {
    loc.searchParams.delete("returnUrl");
    return loc.toString();
  }
  return loc.href;
}

function currentPageLocation(): string {
  return pageLocationFor(window.location.href);
}

/** Pushes the current page_location (without returnUrl) after a client-side navigation. */
export function trackPageLocation(): void {
  if (typeof window === "undefined") return;
  window.dataLayer = window.dataLayer || [];
  const layer = window.dataLayer as unknown[];
  const granted = consentEntries(layer)
    .filter(([k]) => k === "update")
    .pop()?.[1];
  if (granted?.analytics_storage !== "granted") return;
  layer.push({
    page_location: currentPageLocation(),
    page_referrer: scrubReferrer(document.referrer),
  });
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

    const pageLocation = currentPageLocation();
    const lastPage = [...layer]
      .reverse()
      .find(
        (e) =>
          !!e &&
          typeof e === "object" &&
          typeof (e as { page_location?: unknown }).page_location === "string",
      ) as { page_location: string } | undefined;
    if (lastPage?.page_location !== pageLocation) {
      layer.push({
        page_location: pageLocation,
        page_referrer: scrubReferrer(document.referrer),
      });
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
