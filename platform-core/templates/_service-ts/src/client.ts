/**
 * Connect-ES client factory — the edge's ONLY outbound path to the platform.
 *
 * ARCHITECTURE Rule 1: the frontend/edge talks to the **Gateway**, never
 * directly to owned-DB backend services. This module builds Connect clients
 * pointed at GATEWAY_URL; every RPC below is a call to the Gateway, which fans
 * out to team-domain / team-ai / etc. over gRPC.
 *
 * Transport: gRPC (createGrpcTransport) so the hop stays binary gRPC end-to-end.
 * If the Gateway speaks the Connect protocol instead, swap to
 * createConnectTransport from the same package — nothing else changes.
 *
 * NOTE: the `../generated/**` imports below require `npm run proto` (they do not
 * exist until you vendor the proto module and generate — see proto-vendor/).
 * They are commented so this seed type-checks before generation; uncomment once
 * `generated/` exists.
 */

import { createGrpcTransport } from "@connectrpc/connect-node";
import type { Transport } from "@connectrpc/connect";
import { config } from "./config.js";
import { authInterceptor } from "./auth.js";

/**
 * Build the transport to the Gateway. Attaches the auth + trace-propagation
 * interceptor (ADR-0003 / ADR-0004) so every client made from it carries the
 * service bearer token and correlation headers.
 */
export function createGatewayTransport(): Transport {
  return createGrpcTransport({
    baseUrl: config.GATEWAY_URL,
    // httpVersion is "2" for gRPC. connect-node manages the HTTP/2 session.
    interceptors: [authInterceptor({ token: config.AUTH_BEARER_TOKEN })],
  });
}

/** Shared transport singleton for the process. */
export const gatewayTransport: Transport = createGatewayTransport();

// ── Generated-client wiring — requires `npm run proto` ──────────────────────
//
// import { createPromiseClient } from "@connectrpc/connect";
// import { SearchService } from "../generated/platform/search/v1/search_connect.js";
// import { ListingService } from "../generated/platform/listing/v1/listing_connect.js";
//
// /** Typed client for platform.search.v1.SearchService (owned by team-ai). */
// export const searchClient = createPromiseClient(SearchService, gatewayTransport);
//
// /** Typed client for platform.listing.v1.ListingService (owned by team-domain). */
// export const listingClient = createPromiseClient(ListingService, gatewayTransport);
//
// /**
//  * Example typed call: search listings through the Gateway. Mirrors the shape a
//  * BFF route (src/server.ts) would call and hand to an SSR page.
//  *
//  * @param query    free-text / semantic query
//  * @param pageSize server clamps to a max; 0 = server default
//  */
// export async function searchListings(query: string, pageSize = 20) {
//   const res = await searchClient.searchListings({
//     query,
//     page: { cursor: "", pageSize },
//     filters: {},
//   });
//   // res.hits: { listingId: string; score: number }[]
//   // res.page: { nextCursor: string; total: bigint }
//   return res.hits.map((h) => ({ listingId: h.listingId, score: h.score }));
// }
