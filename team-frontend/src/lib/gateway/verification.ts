/**
 * Server-only gateway module for team-verification (VerificationService), reached
 * — like every other domain — ONLY through the gateway (ARCHITECTURE Rule 1). A
 * per-request client is built from the caller's httpOnly `session` cookie,
 * mirroring sessions.ts / promotion.ts (VerificationService is not part of the
 * shared makeClients bundle, so it stays a self-contained gateway module). No
 * business logic here: team-verification owns KYC review; this module forwards
 * and maps proto → plain view types.
 */
import "server-only";

import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-node";

import { VerificationService } from "@/generated/platform/verification/v1/verification_connect.js";

import { authInterceptor } from "./auth.js";
import { gatewayConfig } from "./config.js";
import { getToken } from "./session.js";

function verification() {
  const transport = createConnectTransport({
    baseUrl: gatewayConfig.gatewayUrl,
    httpVersion: "1.1",
    interceptors: [authInterceptor({ token: getToken() })],
  });
  return createPromiseClient(VerificationService, transport);
}

/** Public verified flags for the given seller ids; absent ids are not verified. */
export async function getPublicBadges(
  userIds: string[],
): Promise<Map<string, boolean>> {
  const res = await verification().getPublicBadges({ userIds });
  return new Map(res.badges.map((b) => [b.userId, b.verified]));
}
