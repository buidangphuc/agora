/**
 * Edge → Gateway configuration, read once from the environment. The frontend
 * talks ONLY to the gateway (ARCHITECTURE Rule 1); these are the knobs for that
 * single outbound hop.
 */
export interface GatewayConfig {
  readonly gatewayUrl: string;
  readonly serviceName: string;
}

function isHttpUrl(value: string): boolean {
  try {
    const u = new URL(value);
    return u.protocol === "http:" || u.protocol === "https:";
  } catch {
    return false;
  }
}

export function loadGatewayConfig(
  env: NodeJS.ProcessEnv = process.env,
): GatewayConfig {
  const gatewayUrl = env.GATEWAY_URL?.trim() || "http://127.0.0.1:8080";
  if (!isHttpUrl(gatewayUrl)) {
    throw new Error(`GATEWAY_URL must be an http(s) URL, got "${gatewayUrl}"`);
  }
  return Object.freeze({
    gatewayUrl,
    serviceName: env.OTEL_SERVICE_NAME?.trim() || "team-frontend",
  });
}

export const gatewayConfig: GatewayConfig = loadGatewayConfig();
