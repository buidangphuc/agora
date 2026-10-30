/**
 * Pure (runtime-agnostic: Node, Edge middleware, jsdom) check of a session JWT's
 * `exp` claim. NOT a verification — the Gateway is the verifier (ADR-0003). Its
 * only job is to avoid sending the Gateway a bearer we already know it will
 * reject: the Gateway answers Unauthenticated to any presented-but-invalid
 * token, even on public routes (RFC 6750 §3.1), so a stale cookie would
 * otherwise break anonymous-grade browsing.
 */

export type TokenState = "usable" | "expired" | "malformed";

// Tolerate small clock skew between the browser/Next host and the Gateway.
const SKEW_SECONDS = 5;

export function tokenState(token: string, nowMs = Date.now()): TokenState {
  const parts = token.split(".");
  if (parts.length !== 3) return "malformed";
  try {
    const b64 = parts[1].replace(/-/g, "+").replace(/_/g, "/");
    const padded = b64 + "=".repeat((4 - (b64.length % 4)) % 4);
    const payload = JSON.parse(atob(padded)) as { exp?: unknown };
    if (typeof payload !== "object" || payload === null) return "malformed";
    if (
      typeof payload.exp === "number" &&
      nowMs / 1000 >= payload.exp - SKEW_SECONDS
    ) {
      return "expired";
    }
    return "usable";
  } catch {
    return "malformed";
  }
}

export function isUsableToken(token: string, nowMs = Date.now()): boolean {
  return tokenState(token, nowMs) === "usable";
}
