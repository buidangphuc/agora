export interface GatewayConfig {
  readonly gatewayUrl: string;
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
  env: Record<string, string | undefined> = process.env,
): GatewayConfig {
  const gatewayUrl = env.GATEWAY_URL?.trim() || "http://127.0.0.1:8080";
  if (!isHttpUrl(gatewayUrl)) {
    throw new Error(`GATEWAY_URL must be an http(s) URL, got "${gatewayUrl}"`);
  }
  return Object.freeze({ gatewayUrl });
}
