import { InMemoryProvider, OpenFeature } from "@openfeature/server-sdk";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { FLAG_CHECKOUT_ENABLED, getFlag, isCheckoutEnabled } from "./index.js";

// Replace the real Flipt provider registration with a no-op; each test sets an
// in-memory (fake) provider directly, exercising the real OpenFeature evaluation
// path used in production.
vi.mock("./provider.js", () => ({
  registerFliptProvider: vi.fn(async () => {}),
}));

async function setFlag(value: boolean): Promise<void> {
  await OpenFeature.setProviderAndWait(
    new InMemoryProvider({
      [FLAG_CHECKOUT_ENABLED]: {
        variants: { on: true, off: false },
        defaultVariant: value ? "on" : "off",
        disabled: false,
      },
    }),
  );
}

describe("checkout kill-switch flag (server-side)", () => {
  beforeEach(async () => {
    await OpenFeature.setProviderAndWait(new InMemoryProvider({}));
  });

  it("allows checkout when the flag is ON", async () => {
    await setFlag(true);
    await expect(isCheckoutEnabled()).resolves.toBe(true);
  });

  it("blocks checkout when the flag is OFF", async () => {
    await setFlag(false);
    await expect(isCheckoutEnabled()).resolves.toBe(false);
  });

  it("fails open to the default when the flag is unavailable", async () => {
    // No flag configured → evaluation error → getFlag returns the caller default.
    await OpenFeature.setProviderAndWait(new InMemoryProvider({}));
    await expect(getFlag(FLAG_CHECKOUT_ENABLED, true)).resolves.toBe(true);
    await expect(isCheckoutEnabled()).resolves.toBe(true);
  });
});
