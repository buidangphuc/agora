import { Code, ConnectError } from "@connectrpc/connect";
import { cookies } from "next/headers";
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

const initial: AuthState = {};

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

    const jar = cookies();
    expect(jar.set).toHaveBeenCalledWith(
      "session",
      "jwt-123",
      expect.objectContaining({ httpOnly: true, path: "/" }),
    );
    expect(redirect).toHaveBeenCalledWith("/");
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
      error: "Tên đăng nhập hoặc mật khẩu không chính xác.",
    });
    expect(redirect).not.toHaveBeenCalled();
  });

  it("returns a connection error for non-Connect failures", async () => {
    stubAuth({ login: vi.fn().mockRejectedValue(new Error("dns")) });
    const res = await loginAction(
      initial,
      form({ username: "x", password: "y" }),
    );
    expect(res).toEqual({ error: "Không thể kết nối đến máy chủ xác thực." });
  });

  it("errors when the service returns no token", async () => {
    stubAuth({ login: vi.fn().mockResolvedValue({ result: { token: "" } }) });
    const res = await loginAction(
      initial,
      form({ username: "x", password: "y" }),
    );
    expect(res).toEqual({ error: "Không nhận được phiên đăng nhập." });
  });
});

describe("registerAction", () => {
  it("validates username/password length before calling the gateway", async () => {
    const res = await registerAction(
      initial,
      form({ username: "ab", password: "123" }),
    );
    expect(res.error).toContain("≥ 3 ký tự");
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
    expect(cookies().set).toHaveBeenCalledWith(
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
    expect(res).toEqual({ error: "Tên đăng nhập đã tồn tại." });
  });
});

describe("logoutAction", () => {
  it("clears the session cookie and redirects home", async () => {
    await logoutAction();
    expect(cookies().delete).toHaveBeenCalledWith("session");
    expect(redirect).toHaveBeenCalledWith("/");
  });
});
