import { Code, ConnectError } from "@connectrpc/connect";
import { cookies, headers } from "next/headers";
import { redirect } from "next/navigation";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { makeClients } from "@/lib/gateway/client";
import { refreshSession } from "@/lib/gateway/refresh";

import { type AuthState, loginAction, logoutAction } from "./actions";

vi.mock("@/lib/gateway/client", () => ({ makeClients: vi.fn() }));
vi.mock("@/lib/gateway/refresh", () => ({ refreshSession: vi.fn() }));
vi.mock("@/lib/gateway/revoke", async (importOriginal) => {
  const orig = await importOriginal<typeof import("@/lib/gateway/revoke")>();
  return { ...orig, revokeOwnSession: vi.fn(orig.revokeOwnSession) };
});

const seen = vi.hoisted(() => ({
  calls: [] as {
    headers: Headers;
    timeoutMs: number | undefined;
    message: unknown;
    method: string;
  }[],
  reply: (() => ({})) as () => unknown,
}));

vi.mock("@connectrpc/connect-node", () => ({
  createConnectTransport: (opts: {
    interceptors?: ((
      next: (r: unknown) => Promise<unknown>,
    ) => (r: unknown) => Promise<unknown>)[];
    defaultTimeoutMs?: number;
  }) => ({
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
        return {
          message: seen.reply(),
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

const initial: AuthState = { ok: false };

function form(fields: Record<string, string>): FormData {
  const fd = new FormData();
  for (const [k, v] of Object.entries(fields)) fd.set(k, v);
  return fd;
}

function stubAuth(login: ReturnType<typeof vi.fn>) {
  vi.mocked(makeClients).mockReturnValue({ auth: { login } } as never);
}

beforeEach(() => vi.clearAllMocks());

const b64 = (o: object) => Buffer.from(JSON.stringify(o)).toString("base64url");
const jwt = (claims: object) => `h.${b64(claims)}.s`;
const future = () => Math.floor(Date.now() / 1000) + 3600;

async function freshJar() {
  const jar = await cookies();
  for (const n of [
    "admin_session",
    "admin_refresh",
    "session",
    "agora_refresh",
  ])
    jar.delete(n);
  vi.mocked(jar.delete).mockClear();
  vi.mocked(jar.set).mockClear();
  return jar;
}

afterEach(() => {
  vi.mocked(headers).mockImplementation(
    () => new Map<string, string>() as never,
  );
});

describe("admin loginAction", () => {
  it("stores strict admin cookies and redirects to the return url", async () => {
    const jar = await freshJar();
    stubAuth(
      vi.fn().mockResolvedValue({
        result: { token: "T1", refreshToken: "R1" },
      }),
    );
    await loginAction(
      initial,
      form({ username: "admin", password: "pw", returnUrl: "/admin/users" }),
    );
    expect(jar.set).toHaveBeenCalledWith(
      "admin_session",
      "T1",
      expect.objectContaining({
        httpOnly: true,
        sameSite: "strict",
        maxAge: 1800,
      }),
    );
    expect(jar.set).toHaveBeenCalledWith(
      "admin_refresh",
      "R1",
      expect.objectContaining({ sameSite: "strict", maxAge: 28800 }),
    );
    expect(jar.set).not.toHaveBeenCalledWith(
      "session",
      expect.anything(),
      expect.anything(),
    );
    expect(redirect).toHaveBeenCalledWith("/admin/users");
  });

  it("uses __Host- cookie names behind HTTPS", async () => {
    const jar = await freshJar();
    vi.mocked(headers).mockImplementation(
      () => new Headers({ "x-forwarded-proto": "https" }) as never,
    );
    stubAuth(
      vi.fn().mockResolvedValue({
        result: { token: "T1", refreshToken: "R1" },
      }),
    );
    await loginAction(initial, form({ username: "admin", password: "pw" }));
    expect(jar.set).toHaveBeenCalledWith(
      "__Host-admin_session",
      "T1",
      expect.objectContaining({ secure: true }),
    );
  });

  it("names the empty fields without calling the gateway", async () => {
    const login = vi.fn();
    stubAuth(login);
    const state = await loginAction(initial, form({ username: "" }));
    expect(state.ok).toBe(false);
    expect(state.fields?.username).toBeTruthy();
    expect(state.fields?.password).toBeTruthy();
    expect(login).not.toHaveBeenCalled();
  });

  it("maps Unauthenticated to an invalid-credentials error", async () => {
    const jar = await freshJar();
    stubAuth(
      vi.fn().mockRejectedValue(new ConnectError("bad", Code.Unauthenticated)),
    );
    const state = await loginAction(
      initial,
      form({ username: "admin", password: "x" }),
    );
    expect(state.error).toBe("Tên đăng nhập hoặc mật khẩu không chính xác.");
    expect(jar.set).not.toHaveBeenCalled();
  });

  it("does not echo the backend message", async () => {
    stubAuth(
      vi.fn().mockRejectedValue(new ConnectError("db secret", Code.Internal)),
    );
    const state = await loginAction(
      initial,
      form({ username: "admin", password: "x" }),
    );
    expect(state.error).not.toContain("db secret");
  });

  it("ignores an unsafe return url", async () => {
    await freshJar();
    stubAuth(vi.fn().mockResolvedValue({ result: { token: "T1" } }));
    await loginAction(
      initial,
      form({ username: "admin", password: "pw", returnUrl: "//evil.test" }),
    );
    expect(redirect).toHaveBeenCalledWith("/");
  });
});

describe("admin logoutAction", () => {
  beforeEach(() => {
    seen.calls.length = 0;
    seen.reply = () => ({});
  });

  it("revokes the admin session and clears only the admin cookies", async () => {
    const jar = await freshJar();
    const tok = jwt({ sub: "u1", sid: "sess-A", exp: future() });
    jar.set("admin_session", tok);
    jar.set("admin_refresh", "rt");
    jar.set("session", "buyer");

    await logoutAction();

    expect(seen.calls[0]?.method).toBe("RevokeSession");
    expect(seen.calls[0]?.headers.get("authorization")).toBe(`bearer ${tok}`);
    expect(refreshSession).not.toHaveBeenCalled();
    expect(jar.delete).toHaveBeenCalledWith("admin_session");
    expect(jar.delete).toHaveBeenCalledWith("admin_refresh");
    expect(jar.delete).not.toHaveBeenCalledWith("session");
    expect(redirect).toHaveBeenCalledWith("/login");
  });

  it("still clears the cookies when revoke fails", async () => {
    seen.reply = () => {
      throw new ConnectError("boom", Code.Unavailable);
    };
    const jar = await freshJar();
    jar.set("admin_session", jwt({ sid: "sess-A", exp: future() }));
    await expect(logoutAction()).resolves.toBeUndefined();
    expect(jar.delete).toHaveBeenCalledWith("admin_session");
    expect(jar.delete).toHaveBeenCalledWith("admin_refresh");
  });
});
