/**
 * Feature-flag configuration, read once from the environment. Flags are read
 * SERVER-SIDE only (see ./index.ts); the browser never learns FLIPT_ADDR.
 */
export interface FlagsConfig {
  /** When false, evaluations always return the caller's default (fail-open). */
  readonly enabled: boolean;
  /**
   * Flipt endpoint. The JS OpenFeature Flipt provider talks to Flipt's REST/HTTP
   * API, so this is normalized to an http(s) URL. FLIPT_ADDR may be given as a
   * bare "host:port" (e.g. "flipt:8080") or a full URL.
   *
   * NOTE (apply-time): team-order (Go) uses the Flipt gRPC endpoint (:9000); the
   * JS provider here uses the REST endpoint (:8080). Point FLIPT_ADDR at the HTTP
   * port for the frontend.
   */
  readonly fliptUrl: string;
}

function normalizeUrl(raw: string): string {
  const trimmed = raw.trim();
  if (/^https?:\/\//.test(trimmed)) {
    return trimmed;
  }
  return `http://${trimmed}`;
}

export function loadFlagsConfig(
  env: NodeJS.ProcessEnv = process.env,
): FlagsConfig {
  const fliptUrl = normalizeUrl(env.FLIPT_ADDR?.trim() || "flipt:8080");
  const enabled = (env.FEATURE_FLAGS_ENABLED?.trim() || "true") !== "false";
  return Object.freeze({ enabled, fliptUrl });
}

export const flagsConfig: FlagsConfig = loadFlagsConfig();
