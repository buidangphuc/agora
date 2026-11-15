import "server-only";

import { createHash } from "node:crypto";

import { OpenFeature } from "@openfeature/server-sdk";

import { loadFlagsConfig } from "./config.js";
import { registerFliptProvider } from "./provider.js";

export const VARIANT_TIMEOUT_MS = 50;

const config = loadFlagsConfig();
let providerReady: Promise<void> | null = null;

function ensureProvider(): Promise<void> {
  if (!providerReady) {
    providerReady = registerFliptProvider(config).catch(() => {
      console.warn("[flags] Flipt provider unavailable");
    });
  }
  return providerReady;
}

function hashEntity(entityId: string): string {
  const salt = process.env.FLAG_ENTITY_SALT ?? "";
  return createHash("sha256").update(`${salt}${entityId}`).digest("hex");
}

export async function getVariant(
  flag: string,
  entityId: string,
): Promise<string | null> {
  if (!config.enabled || !entityId) return null;
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    const evaluation = (async () => {
      await ensureProvider();
      const value = await OpenFeature.getClient("team-frontend").getStringValue(
        flag,
        "",
        { targetingKey: hashEntity(entityId) },
      );
      return value || null;
    })();
    evaluation.catch(() => {});
    const timeout = new Promise<null>((resolve) => {
      timer = setTimeout(() => resolve(null), VARIANT_TIMEOUT_MS);
    });
    return await Promise.race([evaluation, timeout]);
  } catch {
    console.warn(`[flags] variant evaluation of "${flag}" failed`);
    return null;
  } finally {
    if (timer) clearTimeout(timer);
  }
}
