import { beforeEach, describe, expect, it, vi } from "vitest";

// Fake the connect-node transport at the true boundary: it runs the interceptors
// the helper configured over a fake request so the headers that would reach the
// gateway are observable.
const seen = vi.hoisted(() => ({
  calls: [] as { headers: Headers; message: unknown; method: string }[],
}));

vi.mock("@connectrpc/connect-node", () => ({
  createConnectTransport: (opts: {
    interceptors?: ((next: (r: unknown) => Promise<unknown>) => (r: unknown) => Promise<unknown>)[];
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
        return { message: { sessions: [], events: [] }, header: new Headers(), trailer: new Headers() };
      };
      let fn = terminal as (r: never) => Promise<unknown>;
      for (const i of [...(opts.interceptors ?? [])].reverse()) {
        fn = i(fn as never) as never;
      }
      return (await fn({ header: new Headers(header), service, method, message } as never)) as { message: unknown };
    },
    stream: vi.fn(),
  }),
}));

vi.mock("./session.js", () => ({ getToken: () => "user-access" }));

import { listLoginHistory, listSessions, revokeSession } from "./sessions";

const CLIENT = { ip: "203.0.113.9", userAgent: "AgoraTest/2" };

beforeEach(() => {
  seen.calls.length = 0;
});

describe("sessions gateway module", () => {
  it("Session client context: revoke another device forwards IP and UA - RevokeSession carries bearer, X-Forwarded-For and User-Agent", async () => {
    await revokeSession("sid-9", CLIENT);

    expect(seen.calls).toHaveLength(1);
    const c = seen.calls[0]!;
    expect(c.method).toBe("RevokeSession");
    expect(c.message).toEqual(expect.objectContaining({ sessionId: "sid-9" }));
    expect(c.headers.get("authorization")).toBe("bearer user-access");
    expect(c.headers.get("x-forwarded-for")).toBe("203.0.113.9");
    expect(c.headers.get("user-agent")).toBe("AgoraTest/2");
  });

  it("Session client context: revoke another device forwards IP and UA - ListSessions carries neither header", async () => {
    await listSessions();

    const c = seen.calls[0]!;
    expect(c.method).toBe("ListSessions");
    expect(c.headers.get("authorization")).toBe("bearer user-access");
    expect(c.headers.has("x-forwarded-for")).toBe(false);
    expect(c.headers.has("user-agent")).toBe(false);
  });

  it("Session client context: revoke another device forwards IP and UA - ListLoginHistory carries neither header", async () => {
    await listLoginHistory();

    const c = seen.calls[0]!;
    expect(c.method).toBe("ListLoginHistory");
    expect(c.headers.has("x-forwarded-for")).toBe(false);
    expect(c.headers.has("user-agent")).toBe(false);
  });

  it("Session client context: missing headers (boundary) - revokeSession without client context sends neither header", async () => {
    await revokeSession("sid-9");

    const c = seen.calls[0]!;
    expect(c.headers.get("authorization")).toBe("bearer user-access");
    expect(c.headers.has("x-forwarded-for")).toBe(false);
    expect(c.headers.has("user-agent")).toBe(false);
  });
});
