import { beforeEach, describe, expect, it, vi } from "vitest";

// Fake the connect-node transport at the true boundary: it runs the interceptors
// the helper configured over a fake request so the headers that would reach the
// gateway are observable.
const seen = vi.hoisted(() => ({
  calls: [] as { headers: Headers; message: unknown; method: string }[],
}));

vi.mock("@connectrpc/connect-node", () => ({
  createConnectTransport: (opts: {
    interceptors?: ((
      next: (r: unknown) => Promise<unknown>,
    ) => (r: unknown) => Promise<unknown>)[];
  }) => ({
    async unary(
      service: unknown,
      method: { name: string },
      _signal: unknown,
      _timeoutMs: unknown,
      header: HeadersInit | undefined,
      message: unknown,
    ) {
      const terminal = async (req: { header: Headers }) => {
        seen.calls.push({ headers: req.header, message, method: method.name });
        return {
          message: { sessions: [], events: [] },
          header: new Headers(),
          trailer: new Headers(),
        };
      };
      let fn = terminal as (r: never) => Promise<unknown>;
      for (const i of [...(opts.interceptors ?? [])].reverse()) {
        fn = i(fn as never) as never;
      }
      return (await fn({
        header: new Headers(header),
        service,
        method,
        message,
      } as never)) as { message: unknown };
    },
    stream: vi.fn(),
  }),
}));

import { revokeOwnSession } from "./revoke";

beforeEach(() => {
  seen.calls.length = 0;
});

describe("revokeOwnSession", () => {
  it("Session client context: logout refresh and revoke forward IP and UA - RevokeSession carries bearer, X-Forwarded-For and User-Agent", async () => {
    await revokeOwnSession("sid-1", "fresh-access", {
      ip: "203.0.113.9",
      userAgent: "AgoraTest/2",
    });

    expect(seen.calls).toHaveLength(1);
    const c = seen.calls[0];
    if (!c) throw new Error("no gateway call recorded");
    expect(c.method).toBe("RevokeSession");
    expect(c.message).toEqual(expect.objectContaining({ sessionId: "sid-1" }));
    expect(c.headers.get("authorization")).toBe("bearer fresh-access");
    expect(c.headers.get("x-forwarded-for")).toBe("203.0.113.9");
    expect(c.headers.get("user-agent")).toBe("AgoraTest/2");
  });

  it("Session client context: missing headers (boundary) - no client context sends neither header, bearer kept", async () => {
    await revokeOwnSession("sid-1", "fresh-access");

    const c = seen.calls[0];
    if (!c) throw new Error("no gateway call recorded");
    expect(c.headers.get("authorization")).toBe("bearer fresh-access");
    expect(c.headers.has("x-forwarded-for")).toBe(false);
    expect(c.headers.has("user-agent")).toBe(false);
  });

  it("Session client context: logout refresh and revoke forward IP and UA - partial context forwards only what is present", async () => {
    await revokeOwnSession("sid-1", "fresh-access", {
      userAgent: "AgoraTest/2",
    });

    const c = seen.calls[0];
    if (!c) throw new Error("no gateway call recorded");
    expect(c.headers.has("x-forwarded-for")).toBe(false);
    expect(c.headers.get("user-agent")).toBe("AgoraTest/2");
  });
});
