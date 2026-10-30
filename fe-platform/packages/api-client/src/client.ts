import "server-only";

import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

import { loadGatewayConfig } from "./config";
import { AnalyticsQueryService } from "./generated/platform/analytics/v1/analytics_connect";
import {
  AddressService,
  AuthService,
  PublicProfileService,
  SessionService,
} from "./generated/platform/identity/v1/identity_connect";
import { ListingService } from "./generated/platform/listing/v1/listing_connect";
import {
  CartService,
  OrderService,
} from "./generated/platform/order/v1/order_connect";
import { SearchService } from "./generated/platform/search/v1/search_connect";
import { anonymousFallbackInterceptor, authInterceptor } from "./interceptors";

export interface GatewayClientOptions {
  token?: string;
  requestId?: string;
  anonymousFallback?: boolean;
  baseUrl?: string;
  fetch?: typeof globalThis.fetch;
}

export function createGatewayClients(opts: GatewayClientOptions = {}) {
  const baseUrl = opts.baseUrl ?? loadGatewayConfig().gatewayUrl;
  const transport = createConnectTransport({
    baseUrl,
    fetch: opts.fetch,
    interceptors: [
      authInterceptor({ token: opts.token, requestId: opts.requestId }),
      ...(opts.anonymousFallback ? [anonymousFallbackInterceptor()] : []),
    ],
  });
  return {
    listing: createPromiseClient(ListingService, transport),
    search: createPromiseClient(SearchService, transport),
    cart: createPromiseClient(CartService, transport),
    order: createPromiseClient(OrderService, transport),
    auth: createPromiseClient(AuthService, transport),
    session: createPromiseClient(SessionService, transport),
    address: createPromiseClient(AddressService, transport),
    publicProfile: createPromiseClient(PublicProfileService, transport),
    analytics: createPromiseClient(AnalyticsQueryService, transport),
  };
}
