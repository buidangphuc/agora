import { Code, ConnectError } from "@connectrpc/connect";
import { cookies, headers } from "next/headers";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { makeClients } from "@/lib/gateway/client";

import {
  type AuthState,
  loginAction,
  logoutAction,
  registerAction,
} from "./actions";

vi.mock("@/lib/gateway/client", () => ({ makeClients: vi.fn() }));

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
