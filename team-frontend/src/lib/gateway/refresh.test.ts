import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

// Fake the connect-node transport at the true boundary: it runs the interceptors
// the helper configured over a fake request, so the headers that would reach the
// gateway are observable, then answers like the gateway.
const seen = vi.hoisted(() => ({
  calls: [] as {
    headers: Headers;
    timeoutMs: number | undefined;
    message: unknown;
    method: string;
  }[],
  transportOpts: undefined as undefined | { defaultTimeoutMs?: number },
  reply: undefined as undefined | (() => unknown),
}));

vi.mock("@connectrpc/connect-node", () => ({
  createConnectTransport: (opts: {
    interceptors?: ((
      next: (r: unknown) => Promise<unknown>,
    ) => (r: unknown) => Promise<unknown>)[];
    defaultTimeoutMs?: number;
  }) => {
    seen.transportOpts = opts;
    return {
      async unary(
        service: unknown,
        method: { name: string },
        _signal: unknown,
        timeoutMs: number | undefined,
        header: HeadersInit | undefined,
        message: unknown,
      ) {
        const terminal = async (req: { header: Headers }) => {
          seen.calls.push({
            headers: req.header,
            timeoutMs: timeoutMs ?? opts.defaultTimeoutMs,
            message,
            method: method.name,
          });
          const out = seen.reply?.();
          return {
            message: out,
            header: new Headers(),
            trailer: new Headers(),
          };
        };
        let fn = terminal as (r: never) => Promise<unknown>;
        for (const i of [...(opts.interceptors ?? [])].reverse()) {
          fn = i(fn as never) as never;
        }
        const res = (await fn({
          header: new Headers(header),
          service,
          method,
          message,
        } as never)) as { message: unknown };
        return res;
      },
      stream: vi.fn(),
    };
  },
}));

import { refreshSession } from "./refresh";

beforeEach(() => {
  seen.calls.length = 0;
  seen.transportOpts = undefined;
  seen.reply = () => ({
    result: { token: "new-access", refreshToken: "new-refresh" },
  });
});

describe("refreshSession", () => {
  it("S12: sends RefreshSession with the refresh token and no Authorization header", async () => {
    await refreshSession("old-refresh");

    expect(seen.calls).toHaveLength(1);
    expect(seen.calls[0]?.method).toBe("RefreshSession");
    expect(seen.calls[0]?.message).toEqual(
      expect.objectContaining({ refreshToken: "old-refresh" }),
    );
    expect(seen.calls[0]?.headers.has("authorization")).toBe(false);
  });

  it("S5: returns the new access and rotated refresh token", async () => {
    const res = await refreshSession("old-refresh");
    expect(res).toEqual(
      expect.objectContaining({
        token: "new-access",
        refreshToken: "new-refresh",
      }),
    );
  });

  it("S19: returns an empty refresh token on a grace replay", async () => {
    seen.reply = () => ({ result: { token: "a", refreshToken: "" } });
    const res = await refreshSession("old-refresh");
    expect(res.token).toBe("a");
    expect(res.refreshToken).toBe("");
  });

  it("S10: applies a 3 s timeout", async () => {
    await refreshSession("old-refresh");
    expect(seen.calls[0]?.timeoutMs).toBe(3000);
  });

  it("S10: rejects with the ConnectError on a gateway failure and does not retry", async () => {
    seen.reply = () => {
      throw new ConnectError("down", Code.Unavailable);
    };
    await expect(refreshSession("old-refresh")).rejects.toMatchObject({
      code: Code.Unavailable,
    });
    expect(seen.calls).toHaveLength(1);
  });

  it("S9/S10: Unauthenticated is surfaced (not retried anonymously)", async () => {
    seen.reply = () => {
      throw new ConnectError("revoked", Code.Unauthenticated);
    };
    await expect(refreshSession("old-refresh")).rejects.toMatchObject({
      code: Code.Unauthenticated,
    });
    expect(seen.calls).toHaveLength(1);
  });
  it("Session client context: middleware refresh forwards IP and UA, no Authorization", async () => {
    await refreshSession("old-refresh", {
      ip: "203.0.113.9",
      userAgent: "AgoraTest/2",
    });

    expect(seen.calls).toHaveLength(1);
    const h = seen.calls[0]?.headers;
    expect(h.get("x-forwarded-for")).toBe("203.0.113.9");
    expect(h.get("user-agent")).toBe("AgoraTest/2");
    expect(h.has("authorization")).toBe(false);
    expect(seen.calls[0]?.message).toEqual(
      expect.objectContaining({ refreshToken: "old-refresh" }),
    );
  });

  it("Session client context: missing headers (boundary) - empty client sets neither header and still succeeds", async () => {
    const res = await refreshSession("old-refresh", {});

    expect(seen.calls).toHaveLength(1);
    const h = seen.calls[0]?.headers;
    expect(h.has("x-forwarded-for")).toBe(false);
    expect(h.has("user-agent")).toBe(false);
    expect(h.has("authorization")).toBe(false);
    expect(res.token).toBe("new-access");
  });

  it("Session client context: missing headers (boundary) - omitted client sets neither header", async () => {
    await refreshSession("old-refresh");
    const h = seen.calls[0]?.headers;
    expect(h.has("x-forwarded-for")).toBe(false);
    expect(h.has("user-agent")).toBe(false);
  });

  it("Session client context: only one field present forwards only that header", async () => {
    await refreshSession("old-refresh", { userAgent: "AgoraTest/2" });
    const h = seen.calls[0]?.headers;
    expect(h.get("user-agent")).toBe("AgoraTest/2");
    expect(h.has("x-forwarded-for")).toBe(false);
  });

  it("Session client context: refresh failure with client context still surfaces Unauthenticated, 3 s timeout kept", async () => {
    seen.reply = () => {
      throw new ConnectError("revoked", Code.Unauthenticated);
    };
    await expect(
      refreshSession("old-refresh", { ip: "203.0.113.9", userAgent: "A/1" }),
    ).rejects.toMatchObject({ code: Code.Unauthenticated });
    expect(seen.calls).toHaveLength(1);
    expect(seen.calls[0]?.timeoutMs).toBe(3000);
  });
});
