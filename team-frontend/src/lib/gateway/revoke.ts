/**
 * Best-effort server-side revoke of the caller's own session on logout.
 * server-only. Own transport: bearer = the (possibly freshly refreshed) access
 * token, one 3 s attempt. The sid is read from the JWT payload without a
 * signature check (the gateway is the verification point).
 */
import "server-only";

import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-node";

import { SessionService } from "@/generated/platform/identity/v1/identity_connect.js";

import { authInterceptor } from "./auth.js";
import { gatewayConfig } from "./config.js";

const REVOKE_TIMEOUT_MS = 3000;

/** The `sid` claim of a JWT, or undefined (legacy token / malformed). */
export function sessionIdOf(token: string): string | undefined {
  try {
    const payload = token.split(".")[1];
    if (!payload) return undefined;
    const claims = JSON.parse(
      Buffer.from(payload, "base64url").toString("utf8"),
    );
    return typeof claims?.sid === "string" && claims.sid
      ? claims.sid
      : undefined;
  } catch {
    return undefined;
  }
}

export async function revokeOwnSession(
  sessionId: string,
  token: string,
): Promise<void> {
  const transport = createConnectTransport({
    baseUrl: gatewayConfig.gatewayUrl,
    httpVersion: "1.1",
    defaultTimeoutMs: REVOKE_TIMEOUT_MS,
    interceptors: [authInterceptor({ token })],
  });
  await createPromiseClient(SessionService, transport).revokeSession({
    sessionId,
  });
}
