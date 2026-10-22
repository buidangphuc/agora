/**
 * Exchanges a refresh token for a new access/refresh pair. server-only.
 * Uses its own transport: no bearer (the access token is expired or absent),
 * no anonymous fallback (a rejected refresh must surface), one 3 s attempt.
 */
import "server-only";

import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-node";

import { AuthService } from "@/generated/platform/identity/v1/identity_connect.js";
import { AuthResult } from "@/generated/platform/identity/v1/identity_pb.js";

import { authInterceptor } from "./auth.js";
import type { ClientContext } from "./client-context.js";
import { gatewayConfig } from "./config.js";

const REFRESH_TIMEOUT_MS = 3000;

export async function refreshSession(
  refreshToken: string,
  client?: ClientContext,
): Promise<AuthResult> {
  const transport = createConnectTransport({
    baseUrl: gatewayConfig.gatewayUrl,
    httpVersion: "1.1",
    defaultTimeoutMs: REFRESH_TIMEOUT_MS,
    interceptors: [authInterceptor({ client })],
  });
  const res = await createPromiseClient(AuthService, transport).refreshSession(
    { refreshToken },
  );
  return res.result ?? new AuthResult();
}
