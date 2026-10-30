import "server-only";

import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

import { loadGatewayConfig } from "./config";
import { ListingService } from "./generated/platform/listing/v1/listing_connect";
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
  };
}
