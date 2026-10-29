import { Code, ConnectError, type Interceptor } from "@connectrpc/connect";

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
