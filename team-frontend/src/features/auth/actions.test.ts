import { Code, ConnectError } from "@connectrpc/connect";
import { cookies, headers } from "next/headers";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { makeClients } from "@/lib/gateway/client";
import { refreshSession } from "@/lib/gateway/refresh";

import {
  type AuthState,
  loginAction,
  logoutAction,
  registerAction,
} from "./actions";

vi.mock("@/lib/gateway/client", () => ({ makeClients: vi.fn() }));
vi.mock("@/lib/gateway/refresh", () => ({ refreshSession: vi.fn() }));

// Fake the connect-node transport at the true boundary: it runs the configured
// interceptors so the bearer that would reach the gateway is observable.
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

function stubAuth(rpcs: {
  login?: ReturnType<typeof vi.fn>;
  register?: ReturnType<typeof vi.fn>;
}) {
  vi.mocked(makeClients).mockReturnValue({
    auth: { login: vi.fn(), register: vi.fn(), ...rpcs },
  } as never);
}

beforeEach(() => vi.clearAllMocks());

describe("loginAction", () => {
  it("stores the session cookie and redirects home on success", async () => {
    stubAuth({
      login: vi.fn().mockResolvedValue({ result: { token: "jwt-123" } }),
    });
    await loginAction(initial, form({ username: "alice", password: "pw" }));

    const jar = await cookies();
    expect(jar.set).toHaveBeenCalledWith(
      "session",
      "jwt-123",
      expect.objectContaining({ httpOnly: true, path: "/" }),
    );
    expect(redirect).toHaveBeenCalledWith("/");
  });

  it("names the empty fields without calling the gateway", async () => {
    const res = await loginAction(
      initial,
      form({ username: "  ", password: "" }),
    );
    expect(res.ok).toBe(false);
    expect(res.fields).toEqual({
      username: "Vui lòng nhập tên đăng nhập.",
      password: "Vui lòng nhập mật khẩu.",
    });
    expect(makeClients).not.toHaveBeenCalled();
    expect(redirect).not.toHaveBeenCalled();
  });

  it("maps Unauthenticated to an invalid-credentials error", async () => {
    stubAuth({
      login: vi
        .fn()
        .mockRejectedValue(new ConnectError("no", Code.Unauthenticated)),
    });
    const res = await loginAction(
      initial,
      form({ username: "x", password: "y" }),
    );
    expect(res).toEqual({
      ok: false,
      error: "Tên đăng nhập hoặc mật khẩu không chính xác.",
      username: "x",
    });
    expect(redirect).not.toHaveBeenCalled();
  });

  it("returns a connection error for non-Connect failures", async () => {
    stubAuth({ login: vi.fn().mockRejectedValue(new Error("dns")) });
    const res = await loginAction(
      initial,
      form({ username: "x", password: "y" }),
    );
    expect(res).toEqual({
      ok: false,
      error: "Không thể kết nối đến máy chủ xác thực.",
      username: "x",
    });
  });

  it("errors when the service returns no token", async () => {
    stubAuth({ login: vi.fn().mockResolvedValue({ result: { token: "" } }) });
    const res = await loginAction(
      initial,
      form({ username: "x", password: "y" }),
    );
    expect(res).toEqual({
      ok: false,
      error: "Không nhận được phiên đăng nhập.",
      username: "x",
    });
  });
});

describe("client context forwarding", () => {
  it("passes the browser ip and user agent to the gateway on login and register", async () => {
    vi.mocked(headers).mockReturnValue(
      new Map([
        ["x-forwarded-for", "203.0.113.7"],
        ["user-agent", "Mozilla/5.0 test"],
      ]) as never,
    );
    stubAuth({
      login: vi.fn().mockResolvedValue({ result: { token: "t" } }),
      register: vi.fn().mockResolvedValue({ result: { token: "t" } }),
    });

    await loginAction(initial, form({ username: "alice", password: "pw" }));
    await registerAction(
      initial,
      form({ username: "alice", password: "pass1234", role: "buyer" }),
    );

    const want = {
      client: { ip: "203.0.113.7", userAgent: "Mozilla/5.0 test" },
    };
    expect(makeClients).toHaveBeenNthCalledWith(1, undefined, want);
    expect(makeClients).toHaveBeenNthCalledWith(2, undefined, want);
  });
});

describe("registerAction", () => {
  it("validates username/password length before calling the gateway", async () => {
    const res = await registerAction(
      initial,
      form({ username: "ab", password: "123" }),
    );
    expect(res.ok).toBe(false);
    expect(res.error).toContain("≥ 3 ký tự");
    expect(res.fields).toEqual({
      username: "Tên đăng nhập tối thiểu 3 ký tự.",
      password: "Mật khẩu tối thiểu 4 ký tự.",
    });
    expect(makeClients).not.toHaveBeenCalled();
  });

  it("registers and redirects on success", async () => {
    stubAuth({
      register: vi.fn().mockResolvedValue({ result: { token: "jwt-9" } }),
    });
    await registerAction(
      initial,
      form({ username: "alice", password: "pass", role: "seller" }),
    );
    expect((await cookies()).set).toHaveBeenCalledWith(
      "session",
      "jwt-9",
      expect.objectContaining({ httpOnly: true }),
    );
    expect(redirect).toHaveBeenCalledWith("/");
  });

  it("maps AlreadyExists to a taken-username error", async () => {
    stubAuth({
      register: vi
        .fn()
        .mockRejectedValue(new ConnectError("dup", Code.AlreadyExists)),
    });
    const res = await registerAction(
      initial,
      form({ username: "alice", password: "pass" }),
    );
    expect(res).toEqual({
      ok: false,
      error: "Tên đăng nhập đã tồn tại.",
      username: "alice",
      fields: { username: "Tên đăng nhập đã tồn tại." },
    });
  });
});

describe("logoutAction", () => {
  it("clears the session cookie and redirects home", async () => {
    await logoutAction();
    expect((await cookies()).delete).toHaveBeenCalledWith("session");
    expect(redirect).toHaveBeenCalledWith("/");
  });
});

describe("refresh cookie (frontend-session-continuity)", () => {
  function setCalls(jar: Awaited<ReturnType<typeof cookies>>) {
    return vi.mocked(jar.set).mock.calls as unknown as [
      string,
      string,
      Record<string, unknown>,
    ][];
  }
  const find = (
    calls: [string, string, Record<string, unknown>][],
    name: string,
  ) => calls.find((c) => c[0] === name);

  it("S1: login sets session (3600) and agora_refresh with matching secure", async () => {
    const jar = await cookies();
    vi.mocked(jar.set).mockClear();
    stubAuth({
      login: vi.fn().mockResolvedValue({
        result: { token: "jwt-1", refreshToken: "rt-1" },
      }),
    });
    await loginAction(initial, form({ username: "alice", password: "pw" }));

    const calls = setCalls(jar);
    const session = find(calls, "session")!;
    const refresh = find(calls, "agora_refresh")!;
    expect(session[1]).toBe("jwt-1");
    expect(session[2]).toEqual(expect.objectContaining({ maxAge: 3600 }));
    expect(refresh[1]).toBe("rt-1");
    expect(refresh[2]).toEqual(
      expect.objectContaining({
        httpOnly: true,
        sameSite: "lax",
        path: "/",
        maxAge: 2592000,
      }),
    );
    expect(refresh[2].secure).toBe(session[2].secure);
    expect(redirect).toHaveBeenCalledWith("/");
  });

  it("S2: register sets both cookies with the same attributes", async () => {
    const jar = await cookies();
    vi.mocked(jar.set).mockClear();
    stubAuth({
      register: vi.fn().mockResolvedValue({
        result: { token: "jwt-2", refreshToken: "rt-2" },
      }),
    });
    await registerAction(
      initial,
      form({ username: "alice", password: "pass", role: "buyer" }),
    );

    const calls = setCalls(jar);
    const session = find(calls, "session")!;
    const refresh = find(calls, "agora_refresh")!;
    expect(session[1]).toBe("jwt-2");
    expect(session[2]).toEqual(expect.objectContaining({ maxAge: 3600 }));
    expect(refresh[1]).toBe("rt-2");
    expect(refresh[2]).toEqual(
      expect.objectContaining({
        httpOnly: true,
        sameSite: "lax",
        path: "/",
        maxAge: 2592000,
      }),
    );
    expect(refresh[2].secure).toBe(session[2].secure);
    expect(redirect).toHaveBeenCalledWith("/");
  });

  it("S3: empty refreshToken sets only session and still redirects (login)", async () => {
    const jar = await cookies();
    vi.mocked(jar.set).mockClear();
    stubAuth({
      login: vi.fn().mockResolvedValue({
        result: { token: "jwt-3", refreshToken: "" },
      }),
    });
    await loginAction(initial, form({ username: "alice", password: "pw" }));

    const calls = setCalls(jar);
    expect(find(calls, "session")![1]).toBe("jwt-3");
    expect(find(calls, "agora_refresh")).toBeUndefined();
    expect(redirect).toHaveBeenCalledWith("/");
  });

  it("S3: empty refreshToken sets only session and still redirects (register)", async () => {
    const jar = await cookies();
    vi.mocked(jar.set).mockClear();
    stubAuth({
      register: vi.fn().mockResolvedValue({
        result: { token: "jwt-4", refreshToken: "" },
      }),
    });
    await registerAction(
      initial,
      form({ username: "alice", password: "pass", role: "buyer" }),
    );

    const calls = setCalls(jar);
    expect(find(calls, "session")![1]).toBe("jwt-4");
    expect(find(calls, "agora_refresh")).toBeUndefined();
    expect(redirect).toHaveBeenCalledWith("/");
  });

  it("S4: failed login sets no cookie and keeps the error message", async () => {
    const jar = await cookies();
    vi.mocked(jar.set).mockClear();
    stubAuth({
      login: vi
        .fn()
        .mockRejectedValue(new ConnectError("no", Code.Unauthenticated)),
    });
    const res = await loginAction(
      initial,
      form({ username: "x", password: "y" }),
    );
    expect(jar.set).not.toHaveBeenCalled();
    expect(res).toEqual({
      ok: false,
      error: "Tên đăng nhập hoặc mật khẩu không chính xác.",
      username: "x",
    });
  });
});

describe("logoutAction revokes the server session (frontend-session-continuity)", () => {
  const b64 = (o: object) =>
    Buffer.from(JSON.stringify(o)).toString("base64url");
  const jwt = (claims: object) => `h.${b64(claims)}.s`;
  const future = () => Math.floor(Date.now() / 1000) + 3600;
  const past = () => Math.floor(Date.now() / 1000) - 3600;

  async function seed(cookiesToSet: Record<string, string>) {
    const jar = await cookies();
    for (const n of ["session", "agora_refresh"]) jar.delete(n);
    for (const [k, v] of Object.entries(cookiesToSet)) jar.set(k, v);
    vi.mocked(jar.delete).mockClear();
    vi.mocked(jar.set).mockClear();
    return jar;
  }
  function expectCleared(jar: Awaited<ReturnType<typeof cookies>>) {
    expect(jar.delete).toHaveBeenCalledWith("session");
    expect(jar.delete).toHaveBeenCalledWith("agora_refresh");
    expect(redirect).toHaveBeenCalledWith("/");
  }

  beforeEach(() => {
    seen.calls.length = 0;
    seen.reply = () => ({});
  });

  it("S13: usable access token with sid -> RevokeSession(sid) with its bearer, both cookies cleared", async () => {
    const tok = jwt({ sub: "u1", sid: "sess-X", exp: future() });
    const jar = await seed({ session: tok, agora_refresh: "rt" });

    await logoutAction();

    expect(seen.calls).toHaveLength(1);
    expect(seen.calls[0]!.method).toBe("RevokeSession");
    expect(seen.calls[0]!.message).toEqual(
      expect.objectContaining({ sessionId: "sess-X" }),
    );
    expect(seen.calls[0]!.headers.get("authorization")).toBe(`bearer ${tok}`);
    expect(seen.calls[0]!.timeoutMs).toBe(3000);
    expect(refreshSession).not.toHaveBeenCalled();
    expectCleared(jar);
  });

  it("S14: revoke failure is swallowed, cookies cleared, redirect", async () => {
    seen.reply = () => {
      throw new ConnectError("boom", Code.Unavailable);
    };
    const jar = await seed({
      session: jwt({ sid: "sess-X", exp: future() }),
      agora_refresh: "rt",
    });

    await expect(logoutAction()).resolves.toBeUndefined();

    expect(seen.calls).toHaveLength(1);
    expectCleared(jar);
  });

  it("S15: expired access + refresh cookie -> refresh first, revoke with fresh sid and bearer", async () => {
    const fresh = jwt({ sub: "u1", sid: "sess-F", exp: future() });
    vi.mocked(refreshSession).mockResolvedValue({
      token: fresh,
      refreshToken: "rt-2",
    } as never);
    const jar = await seed({
      session: jwt({ sid: "sess-OLD", exp: past() }),
      agora_refresh: "rt-1",
    });

    await logoutAction();

    expect(refreshSession).toHaveBeenCalledWith("rt-1");
    expect(seen.calls).toHaveLength(1);
    expect(seen.calls[0]!.method).toBe("RevokeSession");
    expect(seen.calls[0]!.message).toEqual(
      expect.objectContaining({ sessionId: "sess-F" }),
    );
    expect(seen.calls[0]!.headers.get("authorization")).toBe(`bearer ${fresh}`);
    expect(seen.calls[0]!.timeoutMs).toBe(3000);
    expectCleared(jar);
  });

  it("S15: missing access cookie + refresh cookie takes the refresh path", async () => {
    const fresh = jwt({ sid: "sess-F", exp: future() });
    vi.mocked(refreshSession).mockResolvedValue({ token: fresh } as never);
    const jar = await seed({ agora_refresh: "rt-1" });

    await logoutAction();

    expect(refreshSession).toHaveBeenCalledWith("rt-1");
    expect(seen.calls.map((c) => c.method)).toEqual(["RevokeSession"]);
    expectCleared(jar);
  });

  it("S24: RefreshSession fails -> no revoke, cookies cleared, redirect", async () => {
    vi.mocked(refreshSession).mockRejectedValue(
      new ConnectError("no", Code.Unauthenticated),
    );
    const jar = await seed({ agora_refresh: "rt-1" });

    await expect(logoutAction()).resolves.toBeUndefined();

    expect(seen.calls).toHaveLength(0);
    expectCleared(jar);
  });

  it("S24: refreshed token has no sid -> no revoke, cookies cleared", async () => {
    vi.mocked(refreshSession).mockResolvedValue({
      token: jwt({ sub: "u1", exp: future() }),
    } as never);
    const jar = await seed({ agora_refresh: "rt-1" });

    await logoutAction();

    expect(seen.calls).toHaveLength(0);
    expectCleared(jar);
  });

  it("S24: refresh ok but revoke fails -> swallowed, cookies cleared", async () => {
    vi.mocked(refreshSession).mockResolvedValue({
      token: jwt({ sid: "sess-F", exp: future() }),
    } as never);
    seen.reply = () => {
      throw new ConnectError("boom", Code.Internal);
    };
    const jar = await seed({ agora_refresh: "rt-1" });

    await expect(logoutAction()).resolves.toBeUndefined();

    expectCleared(jar);
  });

  it("S25: no cookies -> no gateway call, both cookies deleted, redirect", async () => {
    const jar = await seed({});

    await logoutAction();

    expect(refreshSession).not.toHaveBeenCalled();
    expect(seen.calls).toHaveLength(0);
    expectCleared(jar);
  });

  it("S25: legacy usable token without sid and no refresh cookie -> no revoke, cleared", async () => {
    const jar = await seed({ session: jwt({ sub: "u1", exp: future() }) });

    await logoutAction();

    expect(refreshSession).not.toHaveBeenCalled();
    expect(seen.calls).toHaveLength(0);
    expectCleared(jar);
  });

  it("S13: legacy usable token without sid (with refresh cookie) -> no revoke call, both cleared", async () => {
    const jar = await seed({
      session: jwt({ sub: "u1", exp: future() }),
      agora_refresh: "rt",
    });

    await logoutAction();

    expect(seen.calls).toHaveLength(0);
    expectCleared(jar);
  });

  it("S17: only the caller's own sid is revoked (no user-supplied id)", async () => {
    await seed({
      session: jwt({ sub: "alice", sid: "alice-sess", exp: future() }),
      agora_refresh: "rt",
    });

    await (logoutAction as unknown as (arg?: unknown) => Promise<void>)(
      "attacker-sess",
    );

    expect(seen.calls).toHaveLength(1);
    expect(seen.calls[0]!.message).toEqual(
      expect.objectContaining({ sessionId: "alice-sess" }),
    );
  });
});

describe("returnUrl redirect (frontend-login-return)", () => {
  it("login redirects to returnUrl", async () => {
    stubAuth({
      login: vi.fn().mockResolvedValue({ result: { token: "jwt" } }),
    });
    await loginAction(
      initial,
      form({
        username: "alice",
        password: "pw",
        returnUrl: "/listing/abc?variant=v2",
      }),
    );
    expect(redirect).toHaveBeenCalledTimes(1);
    expect(redirect).toHaveBeenCalledWith("/listing/abc?variant=v2");
  });

  it("login without returnUrl redirects home", async () => {
    stubAuth({
      login: vi.fn().mockResolvedValue({ result: { token: "jwt" } }),
    });
    await loginAction(initial, form({ username: "alice", password: "pw" }));
    expect(redirect).toHaveBeenCalledTimes(1);
    expect(redirect).toHaveBeenCalledWith("/");
  });

  it.each([
    "https://evil.example",
    "//evil",
    "/\\evil",
    "/login",
    "javascript:alert(1)",
  ])("login ignores unsafe returnUrl %s", async (bad) => {
    stubAuth({
      login: vi.fn().mockResolvedValue({ result: { token: "jwt" } }),
    });
    await loginAction(
      initial,
      form({ username: "alice", password: "pw", returnUrl: bad }),
    );
    expect(redirect).toHaveBeenCalledTimes(1);
    expect(redirect).toHaveBeenCalledWith("/");
  });

  it("failed login keeps the form", async () => {
    stubAuth({
      login: vi
        .fn()
        .mockRejectedValue(new ConnectError("no", Code.Unauthenticated)),
    });
    const res = await loginAction(
      initial,
      form({ username: "x", password: "y", returnUrl: "/listing/abc" }),
    );
    expect(res.ok).toBe(false);
    expect(res.error).toBe("Tên đăng nhập hoặc mật khẩu không chính xác.");
    expect(redirect).not.toHaveBeenCalled();
  });

  it("register redirects to returnUrl", async () => {
    stubAuth({
      register: vi.fn().mockResolvedValue({ result: { token: "jwt" } }),
    });
    await registerAction(
      initial,
      form({
        username: "alice",
        password: "pass",
        role: "buyer",
        returnUrl: "/listing/abc",
      }),
    );
    expect(redirect).toHaveBeenCalledTimes(1);
    expect(redirect).toHaveBeenCalledWith("/listing/abc");
  });

  it("register ignores unsafe returnUrl", async () => {
    stubAuth({
      register: vi.fn().mockResolvedValue({ result: { token: "jwt" } }),
    });
    await registerAction(
      initial,
      form({
        username: "alice",
        password: "pass",
        returnUrl: "https://evil.example",
      }),
    );
    expect(redirect).toHaveBeenCalledWith("/");
  });

  it("register already exists keeps the form", async () => {
    stubAuth({
      register: vi
        .fn()
        .mockRejectedValue(new ConnectError("dup", Code.AlreadyExists)),
    });
    const res = await registerAction(
      initial,
      form({ username: "alice", password: "pass", returnUrl: "/listing/abc" }),
    );
    expect(res.ok).toBe(false);
    expect(res.error).toBe("Tên đăng nhập đã tồn tại.");
    expect(redirect).not.toHaveBeenCalled();
  });
});

describe("auth event cookie (loop events)", () => {
  const EVT = "agora_auth_evt";
  const evtCalls = async () =>
    vi.mocked((await cookies()).set).mock.calls.filter((c) => c[0] === EVT);

  it("register success sets the sign_up event cookie", async () => {
    stubAuth({
      register: vi.fn().mockResolvedValue({ result: { token: "jwt-9" } }),
    });
    await registerAction(
      initial,
      form({ username: "alice", password: "pass", role: "buyer" }),
    );
    const jar = await cookies();
    const calls = await evtCalls();
    expect(calls).toHaveLength(1);
    expect(calls[0][1]).toBe("sign_up");
    expect(calls[0][2]).toEqual(
      expect.objectContaining({
        maxAge: 60,
        sameSite: "lax",
        path: "/",
        httpOnly: false,
      }),
    );
    const sets = vi.mocked(jar.set);
    const order = (name: string) =>
      sets.mock.invocationCallOrder[
        sets.mock.calls.findIndex((c) => c[0] === name)
      ];
    expect(order("session")).toBeLessThan(order(EVT));
    expect(order(EVT)).toBeLessThan(
      vi.mocked(redirect).mock.invocationCallOrder[0],
    );
    expect(redirect).toHaveBeenCalledWith("/");
  });

  it("login success sets the login event cookie", async () => {
    stubAuth({
      login: vi.fn().mockResolvedValue({ result: { token: "jwt-123" } }),
    });
    await loginAction(initial, form({ username: "alice", password: "pw" }));
    const jar = await cookies();
    const calls = await evtCalls();
    expect(calls).toHaveLength(1);
    expect(calls[0][1]).toBe("login");
    expect(calls[0][2]).toEqual(
      expect.objectContaining({
        maxAge: 60,
        sameSite: "lax",
        path: "/",
        httpOnly: false,
      }),
    );
    const sets = vi.mocked(jar.set);
    const order = (name: string) =>
      sets.mock.invocationCallOrder[
        sets.mock.calls.findIndex((c) => c[0] === name)
      ];
    expect(order("session")).toBeLessThan(order(EVT));
    expect(order(EVT)).toBeLessThan(
      vi.mocked(redirect).mock.invocationCallOrder[0],
    );
    expect(redirect).toHaveBeenCalledWith("/");
  });

  it("failures never set the event cookie", async () => {
    const login = (
      rpc: ReturnType<typeof vi.fn>,
      fields: Record<string, string>,
    ) => {
      stubAuth({ login: rpc });
      return loginAction(initial, form(fields));
    };
    const register = (
      rpc: ReturnType<typeof vi.fn>,
      fields: Record<string, string>,
    ) => {
      stubAuth({ register: rpc });
      return registerAction(initial, form(fields));
    };
    const ok = { username: "alice", password: "pass1234" };

    await login(vi.fn(), { username: "  ", password: "" });
    await login(
      vi.fn().mockRejectedValue(new ConnectError("no", Code.Unauthenticated)),
      ok,
    );
    await login(vi.fn().mockRejectedValue(new Error("dns")), ok);
    await login(vi.fn().mockResolvedValue({ result: { token: "" } }), ok);

    await register(vi.fn(), { username: "ab", password: "1" });
    await register(
      vi.fn().mockRejectedValue(new ConnectError("dup", Code.AlreadyExists)),
      ok,
    );
    await register(vi.fn().mockRejectedValue(new Error("boom")), ok);
    await register(vi.fn().mockResolvedValue({ result: { token: "" } }), ok);

    expect(await evtCalls()).toHaveLength(0);
    expect(redirect).not.toHaveBeenCalled();
  });
});

describe("login enforcement refusals (frontend-user-enforcement)", () => {
  const REFUSAL =
    "Tài khoản của bạn đã bị khóa. Vui lòng liên hệ bộ phận hỗ trợ.";

  async function loginWith(err: unknown) {
    const jar = await cookies();
    vi.mocked(jar.set).mockClear();
    stubAuth({ login: vi.fn().mockRejectedValue(err) });
    const res = await loginAction(
      initial,
      form({ username: "x", password: "y" }),
    );
    return { res, jar };
  }

  it.each([
    ["suspended", "user suspended until 2026-10-20"],
    ["banned", "user banned"],
  ])(
    "Users UI: Suspended user sees the fixed refusal - maps permission denied to the fixed refusal (%s)",
    async (_n, raw) => {
      const { res, jar } = await loginWith(
        new ConnectError(raw, Code.PermissionDenied),
      );
      expect(res).toEqual({ ok: false, error: REFUSAL, username: "x" });
      expect(res.error).not.toContain("2026");
      expect(res.error).not.toContain(raw);
      expect(jar.set).not.toHaveBeenCalledWith(
        "session",
        expect.anything(),
        expect.anything(),
      );
      expect(redirect).not.toHaveBeenCalled();
    },
  );

  it("Users UI: Wrong password keeps the credentials message - wrong password keeps the credentials message", async () => {
    const { res, jar } = await loginWith(
      new ConnectError("no", Code.Unauthenticated),
    );
    expect(res).toEqual({
      ok: false,
      error: "Tên đăng nhập hoặc mật khẩu không chính xác.",
      username: "x",
    });
    expect(jar.set).not.toHaveBeenCalledWith(
      "session",
      expect.anything(),
      expect.anything(),
    );
  });

  it("Users UI: Other login failures are unchanged - other failures keep the generic message", async () => {
    const { res, jar } = await loginWith(
      new ConnectError("down", Code.Unavailable),
    );
    expect(res.ok).toBe(false);
    expect(res.error?.startsWith("Đăng nhập không thành công:")).toBe(true);
    expect(res.error).not.toBe(REFUSAL);
    expect(jar.set).not.toHaveBeenCalledWith(
      "session",
      expect.anything(),
      expect.anything(),
    );
  });
});
