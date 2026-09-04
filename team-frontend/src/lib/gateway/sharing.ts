/**
 * Server-only gateway module for team-sharing (SharingService), reached — like
 * every other domain — ONLY through the gateway (ARCHITECTURE Rule 1). A
 * per-request client is built from the caller's httpOnly `session` cookie,
 * mirroring sessions.ts / promotion.ts (SharingService is not part of the shared
 * makeClients bundle, so it stays a self-contained gateway module). No business
 * logic here: team-sharing owns short codes/UTM/OG; this module forwards and
 * maps proto → plain view types.
 */
import "server-only";

import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-node";

import { SharingService } from "@/generated/platform/sharing/v1/sharing_connect.js";

import { authInterceptor } from "./auth.js";
import { gatewayConfig } from "./config.js";
import { getToken } from "./session.js";

function sharing() {
  const transport = createConnectTransport({
    baseUrl: gatewayConfig.gatewayUrl,
    httpVersion: "1.1",
    interceptors: [authInterceptor({ token: getToken() })],
  });
  return createPromiseClient(SharingService, transport);
}

export interface ViewShareLink {
  targetType: string;
  targetId: string;
  utm: Record<string, string>;
  ogTitle: string;
  ogDescription: string;
  ogImageUrl: string;
  clickCount: number;
}

/** Create a short link pointing at a target (listing, storefront, etc.). */
export async function createShareLink(
  targetType: string,
  targetId: string,
  utm?: Record<string, string>,
): Promise<string> {
  const res = await sharing().createShareLink({
    targetType,
    targetId,
    utm: utm ?? {},
  });
  return res.shortCode;
}

/** Resolve a short code back to its target plus OG metadata for previews. */
export async function resolveShareLink(
  shortCode: string,
): Promise<ViewShareLink | null> {
  try {
    const res = await sharing().resolveShareLink({ shortCode });
    return {
      targetType: res.targetType,
      targetId: res.targetId,
      utm: res.utm ?? {},
      ogTitle: res.ogMeta?.title ?? "",
      ogDescription: res.ogMeta?.description ?? "",
      ogImageUrl: res.ogMeta?.imageUrl ?? "",
      clickCount: Number(res.clickCount),
    };
  } catch {
    return null;
  }
}
