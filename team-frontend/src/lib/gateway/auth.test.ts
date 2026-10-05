import { Code, ConnectError } from "@connectrpc/connect";
import { describe, expect, it, vi } from "vitest";

import {
  AUTHORIZATION_HEADER,
  REQUEST_ID_HEADER,
  anonymousFallbackInterceptor,
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

describe("anonymousFallbackInterceptor", () => {
  const unauthenticated = () =>
    new ConnectError("bad token", Code.Unauthenticated);

  // Mirror the real chain: auth sets the header, fallback sits inside it.
  async function run(
    token: string | undefined,
    transport: (req: { header: Headers }) => Promise<unknown>,
  ) {
    const req = makeReq();
    const fallback = anonymousFallbackInterceptor()(transport as never);
    return authInterceptor({ token })(fallback as never)(req);
  }

  it("retries once without the bearer when the gateway answers Unauthenticated", async () => {
    const seen: (string | null)[] = [];
    const transport = vi.fn(async (req: { header: Headers }) => {
      seen.push(req.header.get(AUTHORIZATION_HEADER));
      if (req.header.has(AUTHORIZATION_HEADER)) throw unauthenticated();
      return "ok";
    });
    await expect(run("stale", transport)).resolves.toBe("ok");
    expect(seen).toEqual(["bearer stale", null]);
  });

  it("does not retry other errors", async () => {
    const transport = vi.fn(async () => {
      throw new ConnectError("down", Code.Unavailable);
    });
    await expect(run("t", transport)).rejects.toMatchObject({
      code: Code.Unavailable,
    });
    expect(transport).toHaveBeenCalledTimes(1);
  });

  it("does not retry an anonymous call that is itself Unauthenticated", async () => {
    const transport = vi.fn(async () => {
      throw unauthenticated();
    });
    await expect(run(undefined, transport)).rejects.toMatchObject({
      code: Code.Unauthenticated,
    });
    expect(transport).toHaveBeenCalledTimes(1);
  });

  it("surfaces the error if the anonymous retry also fails (single retry)", async () => {
    const transport = vi.fn(async () => {
      throw unauthenticated();
    });
    await expect(run("t", transport)).rejects.toMatchObject({
      code: Code.Unauthenticated,
    });
    expect(transport).toHaveBeenCalledTimes(2);
  });
});
