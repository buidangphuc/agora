/**
 * Server-side feature-flag evaluation (OpenFeature + Flipt provider).
 *
 * `server-only`: this module must never be imported into a client component. The
 * browser never evaluates flags, never loads the flag SDK, and never learns the
 * Flipt endpoint — Server Components / Server Actions call getFlag() and ship
 * already-resolved markup (see ARCHITECTURE Rule 1 exception in design.md).
 *
 * Every evaluation fails open to the caller's default: a Flipt outage must not
 * take checkout down. The authoritative block still lives in team-order.
 */
import "server-only";

import { OpenFeature } from "@openfeature/server-sdk";

import { flagsConfig } from "./config.js";
import { registerFliptProvider } from "./provider.js";

export const FLAG_CHECKOUT_ENABLED = "checkout-enabled";

// Register the provider once, lazily, on first evaluation. Failures are
// swallowed so the default (NoopProvider) stays in place → fail-open.
let providerReady: Promise<void> | null = null;

function ensureProvider(): Promise<void> {
  if (!providerReady) {
    providerReady = registerFliptProvider(flagsConfig).catch((err) => {
      console.warn("[flags] Flipt provider unavailable; failing open", err);
    });
  }
  return providerReady;
}

/**
 * Evaluate a boolean flag on the server, returning defaultValue on any error.
 */
export async function getFlag(
  key: string,
  defaultValue: boolean,
): Promise<boolean> {
  if (!flagsConfig.enabled) {
    return defaultValue;
  }
  try {
    await ensureProvider();
    const client = OpenFeature.getClient("team-frontend");
    return await client.getBooleanValue(key, defaultValue, {
      targetingKey: "team-frontend",
    });
  } catch (err) {
    console.warn(`[flags] evaluation of "${key}" failed; using default`, err);
    return defaultValue;
  }
}

/**
 * Checkout kill-switch. Default ON (true) — fail-open so a Flipt outage never
 * blocks checkout; only a deliberate OFF toggle in Flipt hides the entry point.
 */
export function isCheckoutEnabled(): Promise<boolean> {
  return getFlag(FLAG_CHECKOUT_ENABLED, true);
}
