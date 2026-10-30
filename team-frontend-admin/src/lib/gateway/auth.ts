/**
 * Auth + correlation propagation for the frontend → Gateway hop (ADR-0003/0004).
 * Attaches a static `authorization: bearer <token>` and an `x-request-id` to
 * every outbound RPC. Swapping bearer → JWT/cookie later changes only the token
 * resolved here, not call-sites.
 */
import { Code, ConnectError, type Interceptor } from "@connectrpc/connect";

import type { ClientContext } from "./client-context.js";

export const AUTHORIZATION_HEADER = "authorization";
export const REQUEST_ID_HEADER = "x-request-id";
export const FORWARDED_FOR_HEADER = "x-forwarded-for";
export const USER_AGENT_HEADER = "user-agent";

export interface RequestContext {
  token?: string;
  requestId?: string;
  /**
   * The browser's IP / user agent, for calls that create a session (login,
   * register). Sent as X-Forwarded-For / User-Agent: the gateway honours the former
   * only from this server (its TRUSTED_PROXIES) and stamps both on the call so
   * identity records the real device. Omit everywhere else.
   */
  client?: ClientContext;
}

export function authInterceptor(ctx: RequestContext = {}): Interceptor {
  return (next) => async (req) => {
    const token = ctx.token?.trim();
    if (token) {
      req.header.set(AUTHORIZATION_HEADER, `bearer ${token}`);
    }
    if (ctx.client?.ip) {
      req.header.set(FORWARDED_FOR_HEADER, ctx.client.ip);
    }
    if (ctx.client?.userAgent) {
      req.header.set(USER_AGENT_HEADER, ctx.client.userAgent);
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

/**
 * For PUBLIC read calls only. The Gateway answers Unauthenticated to a bearer it
 * cannot verify (expired between our check and its, rotated key, revoked), even
 * on public RPCs. Retry once without the bearer so a stale session degrades to
 * anonymous browsing instead of a failed page. Must sit AFTER authInterceptor in
 * the chain (inner), so that deleting the header here sticks for the retry.
 * Never use for authenticated RPCs: they must surface the 401.
 */
export function anonymousFallbackInterceptor(): Interceptor {
  return (next) => async (req) => {
    try {
      return await next(req);
    } catch (err) {
      if (
        req.header.has(AUTHORIZATION_HEADER) &&
        ConnectError.from(err).code === Code.Unauthenticated
      ) {
        req.header.delete(AUTHORIZATION_HEADER);
        return next(req);
      }
      throw err;
    }
  };
}
