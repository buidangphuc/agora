/**
 * Client-side auth + context propagation for the edge → Gateway hop.
 *
 * ADR-0003 (auth): the service-to-gateway hop carries a static `authorization:
 * bearer <token>` today. The Gateway resolves it to a `platform.common.v1
 * .Principal`; this edge never mints or parses the principal itself. Swapping
 * bearer → JWT/cookie later changes only what token we attach here + the edge's
 * inbound session resolution — not call-sites.
 *
 * ADR-0004 (observability): every hop propagates W3C `traceparent` (trace
 * context) and bridges `x-request-id` (the human-facing correlation id from the
 * existing repo convention). If the caller already has these (e.g. from an
 * inbound SSR request), we forward them; otherwise we mint a request id so a
 * call is never uncorrelated.
 *
 * The header logic below is deliberately framework-free (plain `Headers`) so it
 * is unit-testable without generated code or a live transport — see
 * tests/auth.test.ts. `authInterceptor` wraps it as a Connect Interceptor.
 */

// Type-only import: erased at compile time, so the pure helpers in this module
// carry no runtime dependency on Connect (keeps the unit test light).
import type { Interceptor } from "@connectrpc/connect";

export const AUTHORIZATION_HEADER = "authorization";
export const REQUEST_ID_HEADER = "x-request-id";
export const TRACEPARENT_HEADER = "traceparent";

/** Inbound/ambient context an edge request may already carry, to propagate on. */
export interface RequestContext {
  /** Bearer token for the gateway hop; omit to send no `authorization`. */
  token?: string;
  /** Existing correlation id (e.g. from the inbound SSR request). */
  requestId?: string;
  /** Existing W3C trace context to continue the trace. */
  traceparent?: string;
}

/** Format a bearer credential. Lowercase scheme to match gRPC metadata norms. */
export function bearer(token: string): string {
  return `bearer ${token}`;
}

/** Mint a fresh correlation id when the caller has none. */
export function newRequestId(): string {
  return crypto.randomUUID();
}

/**
 * Apply edge headers to an outbound request, in place. Pure aside from the
 * (optional) id generation, and returns the effective request id so callers can
 * log/echo it. Behaviour:
 *   - `authorization: bearer <token>` iff a non-empty token is supplied.
 *   - `x-request-id` forwarded if present, else newly minted.
 *   - `traceparent` forwarded only if present (a real tracer would start one).
 */
export function applyEdgeHeaders(
  headers: Headers,
  ctx: RequestContext = {},
): string {
  const token = ctx.token?.trim();
  if (token) {
    headers.set(AUTHORIZATION_HEADER, bearer(token));
  }

  const requestId = ctx.requestId?.trim() || newRequestId();
  headers.set(REQUEST_ID_HEADER, requestId);

  const traceparent = ctx.traceparent?.trim();
  if (traceparent) {
    headers.set(TRACEPARENT_HEADER, traceparent);
  }

  return requestId;
}

/**
 * Connect client interceptor that stamps every outbound RPC with the edge
 * headers above. Attach it on the transport (see src/client.ts).
 *
 * `ctx` supplies process-level defaults (e.g. the service bearer token). For
 * per-request propagation of an inbound `x-request-id`/`traceparent`, thread
 * them via the call's `contextValues`/headers in a real implementation — this
 * seed keeps it to a static context to stay minimal.
 */
export function authInterceptor(ctx: RequestContext = {}): Interceptor {
  return (next) => async (req) => {
    applyEdgeHeaders(req.header, ctx);
    return next(req);
  };
}
