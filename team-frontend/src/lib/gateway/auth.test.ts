import { describe, expect, it, vi } from "vitest";

import {
  AUTHORIZATION_HEADER,
  REQUEST_ID_HEADER,
  authInterceptor,
} from "./auth.js";

// Minimal ConnectRPC-style request carrying a Headers bag; the interceptor only
// touches `req.header`.
function makeReq() {
  return { header: new Headers() } as unknown as Parameters<
    ReturnType<ReturnType<typeof authInterceptor>>
  >[0];
}

describe("authInterceptor", () => {
  it("attaches a bearer authorization header when a token is present", async () => {
    const next = vi.fn(async (req) => req);
    const req = makeReq();

    await authInterceptor({ token: "secret-jwt" })(next)(req);

    expect(req.header.get(AUTHORIZATION_HEADER)).toBe("bearer secret-jwt");
    expect(next).toHaveBeenCalledWith(req);
  });

  it("trims the token before building the header", async () => {
    const next = vi.fn(async (req) => req);
    const req = makeReq();

    await authInterceptor({ token: "  padded  " })(next)(req);

    expect(req.header.get(AUTHORIZATION_HEADER)).toBe("bearer padded");
  });

  it("omits the authorization header for anonymous (no token) calls", async () => {
    const next = vi.fn(async (req) => req);
    const req = makeReq();

    await authInterceptor({})(next)(req);

    expect(req.header.has(AUTHORIZATION_HEADER)).toBe(false);
    // still forwards the request
    expect(next).toHaveBeenCalledOnce();
  });

  it("generates an x-request-id when none is supplied", async () => {
    const next = vi.fn(async (req) => req);
    const req = makeReq();

    await authInterceptor({ token: "t" })(next)(req);

    expect(req.header.get(REQUEST_ID_HEADER)).toBeTruthy();
  });

  it("uses the provided requestId and preserves a pre-set header", async () => {
    const next = vi.fn(async (req) => req);

    const req1 = makeReq();
    await authInterceptor({ requestId: "corr-123" })(next)(req1);
    expect(req1.header.get(REQUEST_ID_HEADER)).toBe("corr-123");

    const req2 = makeReq();
    req2.header.set(REQUEST_ID_HEADER, "already-here");
    await authInterceptor({ requestId: "ignored" })(next)(req2);
    expect(req2.header.get(REQUEST_ID_HEADER)).toBe("already-here");
  });
});
