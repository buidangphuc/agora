export type TokenState = "usable" | "expired" | "malformed";

export interface Principal {
  id: string;
  name: string;
  scopes: string[];
  exp?: number;
}

const SKEW_SECONDS = 5;

function payloadOf(token: string): Record<string, unknown> | null {
  const parts = token.split(".");
  if (parts.length !== 3) return null;
  try {
    const b64 = parts[1].replace(/-/g, "+").replace(/_/g, "/");
    const padded = b64 + "=".repeat((4 - (b64.length % 4)) % 4);
    const payload: unknown = JSON.parse(atob(padded));
    return typeof payload === "object" && payload !== null
      ? (payload as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
}

export function tokenState(token: string, nowMs = Date.now()): TokenState {
  const payload = payloadOf(token);
  if (!payload) return "malformed";
  if (
    typeof payload.exp === "number" &&
    nowMs / 1000 >= payload.exp - SKEW_SECONDS
  ) {
    return "expired";
  }
  return "usable";
}

export function isUsableToken(token: string, nowMs = Date.now()): boolean {
  return tokenState(token, nowMs) === "usable";
}

export function decodePrincipal(token: string): Principal | null {
  const payload = payloadOf(token);
  if (!payload) return null;
  return {
    id: typeof payload.sub === "string" ? payload.sub : "",
    name: typeof payload.name === "string" ? payload.name : "",
    scopes: Array.isArray(payload.scopes)
      ? payload.scopes.filter((s): s is string => typeof s === "string")
      : [],
    exp: typeof payload.exp === "number" ? payload.exp : undefined,
  };
}

export function hasScopes(
  principal: Pick<Principal, "scopes"> | null,
  required: readonly string[],
): boolean {
  if (!principal) return false;
  return required.every((s) => principal.scopes.includes(s));
}
