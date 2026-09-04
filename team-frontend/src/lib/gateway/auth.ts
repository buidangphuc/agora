/**
 * Auth + correlation propagation for the frontend → Gateway hop (ADR-0003/0004).
 * Attaches a static `authorization: bearer <token>` and an `x-request-id` to
 * every outbound RPC. Swapping bearer → JWT/cookie later changes only the token
 * resolved here, not call-sites.
 */
import type { Interceptor } from "@connectrpc/connect";

export const AUTHORIZATION_HEADER = "authorization";
export const REQUEST_ID_HEADER = "x-request-id";

export interface RequestContext {
  token?: string;
  requestId?: string;
}

export function authInterceptor(ctx: RequestContext = {}): Interceptor {
  return (next) => async (req) => {
    const token = ctx.token?.trim();
    if (token) {
      req.header.set(AUTHORIZATION_HEADER, `bearer ${token}`);
    }
    if (!req.header.has(REQUEST_ID_HEADER)) {
      req.header.set(
        REQUEST_ID_HEADER,
        ctx.requestId?.trim() || crypto.randomUUID(),
      );
    }
    return next(req);
  };
}
