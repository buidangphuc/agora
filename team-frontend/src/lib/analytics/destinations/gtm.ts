/**
 * Google Tag Manager Integration.
 * Only loads external GTM script when NEXT_PUBLIC_GTM_ID is explicitly provided.
 * Default is disabled (no external calls in dev/CI/E2E).
 */

export function initGTM(): void {
  if (typeof window === "undefined") return;

  const gtmId = process.env.NEXT_PUBLIC_GTM_ID?.trim();
  if (!gtmId) return;

  // Initialize dataLayer array
  window.dataLayer = window.dataLayer || [];
  window.dataLayer.push({
    "gtm.start": new Date().getTime(),
    event: "gtm.js",
  });

  // Inject GTM script tag
  const script = document.createElement("script");
  script.async = true;
  script.src = `https://www.googletagmanager.com/gtm.js?id=${encodeURIComponent(gtmId)}`;
  document.head.appendChild(script);
}
