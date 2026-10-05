/**
 * Connect clients to the Gateway — the frontend's ONLY outbound path (Rule 1).
 * server-only. Clients are built per request with the caller's token (from the
 * session cookie) so the bearer is scoped to the request, never a process
 * singleton and never shipped to the browser.
 */
import "server-only";

import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-node";

import { AIService } from "@/generated/platform/ai/v1/ai_connect.js";
import { ChatService } from "@/generated/platform/chat/v1/chat_connect.js";
import { EngagementService } from "@/generated/platform/engagement/v1/engagement_connect.js";
import {
  AddressService,
  AuthService,
} from "@/generated/platform/identity/v1/identity_connect.js";
import { ListingService } from "@/generated/platform/listing/v1/listing_connect.js";
import {
  CartService,
  OrderService,
} from "@/generated/platform/order/v1/order_connect.js";
import { PaymentService } from "@/generated/platform/payment/v1/payment_connect.js";
import { RecommendationService } from "@/generated/platform/recommendation/v1/recommendation_connect.js";
import { SearchService } from "@/generated/platform/search/v1/search_connect.js";

import { anonymousFallbackInterceptor, authInterceptor } from "./auth.js";
import type { ClientContext } from "./client-context.js";
import { gatewayConfig } from "./config.js";

// makeClients builds the typed gateway clients, attaching `token` as the bearer
// (omit for anonymous calls — the gateway grants public scopes). The gateway
// rejects a bearer it cannot verify with Unauthenticated, even on public RPCs;
// public read call-sites pass `{ anonymousFallback: true }` to retry once without
// the bearer, so a stale session degrades to anonymous browsing.
//
// `client` carries the browser's IP / user agent for the calls that create a
// session (login, register) — see client-context.ts.
export function makeClients(
  token?: string,
  opts: { anonymousFallback?: boolean; client?: ClientContext } = {},
) {
  const transport = createConnectTransport({
    baseUrl: gatewayConfig.gatewayUrl,
    httpVersion: "1.1",
    interceptors: [
      authInterceptor({ token, client: opts.client }),
      ...(opts.anonymousFallback ? [anonymousFallbackInterceptor()] : []),
    ],
  });
  return {
    search: createPromiseClient(SearchService, transport),
    listing: createPromiseClient(ListingService, transport),
    auth: createPromiseClient(AuthService, transport),
    address: createPromiseClient(AddressService, transport),
    engagement: createPromiseClient(EngagementService, transport),
    cart: createPromiseClient(CartService, transport),
    order: createPromiseClient(OrderService, transport),
    payment: createPromiseClient(PaymentService, transport),
    chat: createPromiseClient(ChatService, transport),
    ai: createPromiseClient(AIService, transport),
    recommendation: createPromiseClient(RecommendationService, transport),
  };
}
