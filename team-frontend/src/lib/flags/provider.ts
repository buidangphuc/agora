/**
 * Flipt provider registration for OpenFeature — SERVER-SIDE only.
 *
 * APPLY-TIME RISK (design.md "Provider package specifics"): `@openfeature/flipt-provider`
 * is the ASSUMED package. Unlike the Go hot path (which needs streaming/in-memory
 * evaluation), the frontend flag read is a UX gate, not a per-request hot path, so
 * this provider's server-side (REST) evaluation is acceptable here. The required
 * BEHAVIOR is: flags are evaluated ONLY on the server; the browser receives
 * already-resolved markup and never learns FLIPT_ADDR. The authoritative checkout
 * block stays in team-order — this is UX only.
 */
import "server-only";

import { FliptProvider } from "@openfeature/flipt-provider";
import { OpenFeature } from "@openfeature/server-sdk";

import type { FlagsConfig } from "./config.js";

// Flipt "default" namespace holds the marketplace flags.
const FLIPT_NAMESPACE = "default";

export async function registerFliptProvider(cfg: FlagsConfig): Promise<void> {
  const provider = new FliptProvider(FLIPT_NAMESPACE, { url: cfg.fliptUrl });
  await OpenFeature.setProviderAndWait(provider);
}
